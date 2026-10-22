package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/starslipay/trade_itg/trade_itg_pb"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

const (
	// maxDetailCount 内存里最多保留的明细条数, 避免大面积不一致时把内存打爆
	maxDetailCount = 200
)

// Severity 不一致的严重等级
type Severity int

const (
	// SeverityWarn 只告警不修复: 无明确修复规则的场景, 需人工介入
	SeverityWarn Severity = iota
	// SeverityCritical 状态单边不一致: 按规则自动修复(EnableRepair=true 时)
	SeverityCritical
)

func (s Severity) String() string {
	if s == SeverityCritical {
		return "CRITICAL"
	}
	return "WARN"
}

// Inconsistency 一条不一致明细
type Inconsistency struct {
	Severity Severity
	Kind     string // 不一致类型, 如 order_success_bill_closed
	Key      string // transaction_id
	Detail   string
	Repaired bool // 本次是否已修复
}

// CheckResult 一轮核对的结果汇总
type CheckResult struct {
	Scenario     string
	Total        int64 // 参与核对的记录数(订单+c2b转账单)
	Consistent   int64
	Skipped      int64 // 待支付订单跳过数(规则: 待支付不处理)
	Inconsistent int64
	Repaired     int64
	Err          string // 查询类错误(不影响其余批次)
	Details      []Inconsistency
	Dropped      int64 // 超过 maxDetailCount 未记录的明细数

	// 参与核对的订单按交易状态分布, 与 t_check_meta 中字段1~4对应
	OrdersTotal   int64 // 订单总数
	OrdersSuccess int64 // 成功订单数
	OrdersClosed  int64 // 失败订单数(已关闭)
	OrdersInit    int64 // 待支付订单数

	// 本轮修复动作计数, 与 t_check_meta 中字段5~6对应
	RepairSupply int64 // 补单成功数(订单被补为成功, 含trade_itg补单和主库补成功)
	RepairClose  int64 // 关单成功数(订单被关闭, 含trade_itg关单和主库关单)

	StartTime     time.Time // 脚本核对开始时间
	EndTime       time.Time // 脚本核对结束时间
	DataStartTime time.Time // 核对数据开始时间(窗口模式下为查询窗口左边界)
	DataEndTime   time.Time // 核对数据结束时间(窗口模式下为查询窗口右边界)
}

func newCheckResult(scenario string) *CheckResult {
	return &CheckResult{
		Scenario:  scenario,
		StartTime: time.Now(),
	}
}

func (r *CheckResult) add(issue Inconsistency) {
	if len(r.Details) >= maxDetailCount {
		r.Dropped++
		return
	}
	r.Details = append(r.Details, issue)
}

func (r *CheckResult) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "=== %s ===\n", r.Scenario)
	fmt.Fprintf(&b, "Duration:     %v\n", r.EndTime.Sub(r.StartTime))
	fmt.Fprintf(&b, "Total:        %d\n", r.Total)
	fmt.Fprintf(&b, "Consistent:   %d\n", r.Consistent)
	fmt.Fprintf(&b, "Skipped:      %d (待支付且无c2b转账单, 不处理)\n", r.Skipped)
	fmt.Fprintf(&b, "Inconsistent: %d\n", r.Inconsistent)
	fmt.Fprintf(&b, "Repaired:     %d\n", r.Repaired)
	fmt.Fprintf(&b, "Orders:       total=%d success=%d closed=%d init=%d\n",
		r.OrdersTotal, r.OrdersSuccess, r.OrdersClosed, r.OrdersInit)
	fmt.Fprintf(&b, "Repair:       supply=%d close=%d\n", r.RepairSupply, r.RepairClose)
	if r.Err != "" {
		fmt.Fprintf(&b, "QueryError:   %s\n", r.Err)
	}
	if r.Dropped > 0 {
		fmt.Fprintf(&b, "More issues not recorded: %d\n", r.Dropped)
	}
	for i, issue := range r.Details {
		repaired := ""
		if issue.Repaired {
			repaired = " repaired"
		}
		fmt.Fprintf(&b, "  %d. [%s] %s key=%s %s%s\n",
			i+1, issue.Severity, issue.Kind, issue.Key, issue.Detail, repaired)
	}
	b.WriteString("====================\n")
	return b.String()
}

type Checker struct {
	cfg         Config
	orderModel  *orderModel                 // 从库读
	orderMaster *orderModel                 // 主库写(终态订单修复用), EnableRepair=false 时为 nil
	billModel   *c2bBillModel               // 从库读
	tradeItg    trade_itg_pb.TradeItgClient // 待支付订单调账用(纯补单/纯关单), EnableRepair=false 时为 nil
	checkRecord *checkRecordModel           // 主库写核对记录, 未配置 CheckRecord.CheckId 时为 nil
	logger      logx.Logger
}

func NewChecker(cfg Config, orderConn, orderMasterConn, accountConn sqlx.SqlConn,
	tradeItg trade_itg_pb.TradeItgClient, checkRecord *checkRecordModel) *Checker {
	var orderMaster *orderModel
	if orderMasterConn != nil {
		orderMaster = newOrderModel(orderMasterConn)
	}
	return &Checker{
		cfg:         cfg,
		orderModel:  newOrderModel(orderConn),
		orderMaster: orderMaster,
		billModel:   newC2BBillModel(accountConn),
		tradeItg:    tradeItg,
		checkRecord: checkRecord,
		logger:      logx.WithContext(context.Background()),
	}
}

// Run 执行一轮核对
// 双向扫描: 订单侧变更 + c2b转账单侧变更, 任一侧近期有变动都触发对该交易ID的比对
// compared 用于一轮内去重, 同一交易ID只比对一次
func (c *Checker) Run(ctx context.Context) *CheckResult {
	res := newCheckResult("order_c2b_reconcile")
	compared := make(map[string]struct{})

	if c.cfg.IsFullScan {
		c.orderFullScan(ctx, res, compared)
		c.billFullScan(ctx, res, compared)
	} else {
		now := time.Now()
		res.DataStartTime = now.Add(time.Duration(c.cfg.TimeStart) * time.Second)
		res.DataEndTime = now.Add(time.Duration(c.cfg.TimeEnd) * time.Second)
		c.orderWindow(ctx, res, compared)
		c.billWindow(ctx, res, compared)
	}

	res.EndTime = time.Now()
	c.saveRecord(ctx, res)
	return res
}

// saveRecord 将本轮核对结果写入 t_check_record
// 字段对应关系(t_check_meta 中的登记):
//
//	field1 订单总数 / field2 成功订单数 / field3 失败订单数 / field4 待支付订单数
//	field5 补单成功数 / field6 关单成功数
//	field7 脚本核对开始时间 / field8 脚本核对结束时间
//	field9 核对数据开始时间 / field10 核对数据结束时间
func (c *Checker) saveRecord(ctx context.Context, res *CheckResult) {
	if c.checkRecord == nil {
		return
	}
	rec := &CheckRecord{
		CheckId:     c.cfg.CheckRecord.CheckId,
		CheckName:   c.cfg.CheckRecord.CheckName,
		Field1Data:  res.OrdersTotal,
		Field2Data:  res.OrdersSuccess,
		Field3Data:  res.OrdersClosed,
		Field4Data:  res.OrdersInit,
		Field5Data:  res.RepairSupply,
		Field6Data:  res.RepairClose,
		Field7Data:  recordTime(res.StartTime),
		Field8Data:  recordTime(res.EndTime),
		Field9Data:  recordTime(res.DataStartTime),
		Field10Data: recordTime(res.DataEndTime),
	}
	if err := c.checkRecord.Insert(ctx, rec); err != nil {
		c.logger.Errorf("save check record failed, check_id: %d, check_name: %s, err: %v",
			rec.CheckId, rec.CheckName, err)
	}
}

// orderWindow 增量核对: 窗口内有变更的订单 → 批量查对应c2b转账单 → 逐条比对
func (c *Checker) orderWindow(ctx context.Context, res *CheckResult, compared map[string]struct{}) {
	rows, err := c.orderModel.FindByWindow(ctx, res.DataStartTime, res.DataEndTime)
	if err != nil {
		res.Err = fmt.Sprintf("query t_order failed: %v", err)
		c.logger.Errorf("orderWindow: %v", err)
		return
	}
	c.checkOrderBatches(ctx, chunk(rows, c.cfg.PageSize), res, compared)
}

// billWindow 增量核对: 窗口内有变更的c2b转账单 → 批量查对应订单 → 逐条比对
func (c *Checker) billWindow(ctx context.Context, res *CheckResult, compared map[string]struct{}) {
	rows, err := c.billModel.FindByWindow(ctx, res.DataStartTime, res.DataEndTime)
	if err != nil {
		res.Err = fmt.Sprintf("query t_c2b_bill failed: %v", err)
		c.logger.Errorf("billWindow: %v", err)
		return
	}
	c.checkBillBatches(ctx, chunk(rows, c.cfg.PageSize), res, compared)
}

// orderFullScan 全量核对: 订单表主键游标分页扫描
// 只处理 update_time 早于 TimeEnd 门槛的记录, 跳过仍在途的数据(如c2b单已写但订单状态尚未更新)
func (c *Checker) orderFullScan(ctx context.Context, res *CheckResult, compared map[string]struct{}) {
	cutoff := c.settledCutoff()
	var afterId string
	for {
		rows, err := c.orderModel.FindByCursor(ctx, afterId, c.cfg.PageSize)
		if err != nil {
			res.Err = fmt.Sprintf("query t_order failed: %v", err)
			c.logger.Errorf("orderFullScan: %v", err)
			return
		}
		if len(rows) == 0 {
			return
		}
		c.checkOrderBatches(ctx, [][]*OrderRow{settledOrders(rows, cutoff)}, res, compared)
		afterId = rows[len(rows)-1].TransactionId
		if int64(len(rows)) < c.cfg.PageSize {
			return
		}
	}
}

// billFullScan 全量核对: c2b转账单表主键游标分页扫描(找出订单侧缺失/不一致的记录)
func (c *Checker) billFullScan(ctx context.Context, res *CheckResult, compared map[string]struct{}) {
	cutoff := c.settledCutoff()
	var afterId string
	for {
		rows, err := c.billModel.FindByCursor(ctx, afterId, c.cfg.PageSize)
		if err != nil {
			res.Err = fmt.Sprintf("query t_c2b_bill failed: %v", err)
			c.logger.Errorf("billFullScan: %v", err)
			return
		}
		if len(rows) == 0 {
			return
		}
		c.checkBillBatches(ctx, [][]*C2BBillRow{settledBills(rows, cutoff)}, res, compared)
		afterId = rows[len(rows)-1].TransactionId
		if int64(len(rows)) < c.cfg.PageSize {
			return
		}
	}
}

// settledCutoff 全量模式下判断记录是否已"落定"的时间门槛
// 复用 TimeEnd 语义(通常为 -30s): update_time 早于该时刻才认为状态已稳定, 避免误判在途数据
func (c *Checker) settledCutoff() time.Time {
	return time.Now().Add(time.Duration(c.cfg.TimeEnd) * time.Second)
}

// settledOrders 过滤出已落定的订单
func settledOrders(rows []*OrderRow, cutoff time.Time) []*OrderRow {
	result := make([]*OrderRow, 0, len(rows))
	for _, row := range rows {
		if row.UpdateTime.Before(cutoff) {
			result = append(result, row)
		}
	}
	return result
}

// settledBills 过滤出已落定的c2b转账单
func settledBills(rows []*C2BBillRow, cutoff time.Time) []*C2BBillRow {
	result := make([]*C2BBillRow, 0, len(rows))
	for _, row := range rows {
		if row.UpdateTime.Before(cutoff) {
			result = append(result, row)
		}
	}
	return result
}

// checkOrderBatches 以订单为基准逐批比对
func (c *Checker) checkOrderBatches(ctx context.Context, batches [][]*OrderRow, res *CheckResult, compared map[string]struct{}) {
	for _, rows := range batches {
		// 本轮已比对过的交易ID不再重复处理(双向扫描会重叠)
		pending := make([]*OrderRow, 0, len(rows))
		for _, row := range rows {
			if _, ok := compared[row.TransactionId]; ok {
				continue
			}
			compared[row.TransactionId] = struct{}{}
			pending = append(pending, row)
		}
		if len(pending) == 0 {
			continue
		}

		ids := make([]string, len(pending))
		for i, row := range pending {
			ids[i] = row.TransactionId
		}
		bills, err := c.billModel.FindByIds(ctx, ids)
		if err != nil {
			res.Err = fmt.Sprintf("query t_c2b_bill by ids failed: %v", err)
			c.logger.Errorf("checkOrderBatches: %v", err)
			return
		}

		for _, order := range pending {
			c.reconcileOne(ctx, order, bills[order.TransactionId], res)
		}
	}
}

// checkBillBatches 以c2b转账单为基准逐批比对
func (c *Checker) checkBillBatches(ctx context.Context, batches [][]*C2BBillRow, res *CheckResult, compared map[string]struct{}) {
	for _, rows := range batches {
		pending := make([]*C2BBillRow, 0, len(rows))
		for _, row := range rows {
			if _, ok := compared[row.TransactionId]; ok {
				continue
			}
			compared[row.TransactionId] = struct{}{}
			pending = append(pending, row)
		}
		if len(pending) == 0 {
			continue
		}

		ids := make([]string, len(pending))
		for i, row := range pending {
			ids[i] = row.TransactionId
		}
		orders, err := c.orderModel.FindByIds(ctx, ids)
		if err != nil {
			res.Err = fmt.Sprintf("query t_order by ids failed: %v", err)
			c.logger.Errorf("checkBillBatches: %v", err)
			return
		}

		for _, bill := range pending {
			c.reconcileOne(ctx, orders[bill.TransactionId], bill, res)
		}
	}
}

// reconcileOne 比对一对 订单/c2b转账单并按规则处理:
//  1. 订单待支付(初始状态) + c2b已成功: 调 trade_itg 纯补单接口(SupplyOrder)将订单补成功
//  2. 订单待支付(初始状态) + c2b已关单: 调 trade_itg 纯关单接口(CloseOrder)将订单关闭
//  3. c2b已关 + 订单成功: 关闭订单(主库条件更新)
//  4. c2b已成功 + 订单已关: 将订单补成功(主库条件更新)
//  5. 订单待支付但无c2b转账单: 支付未发起, 不处理
//  6. 单边缺失(订单成功但无c2b单 / 有c2b单但无订单): 只告警不自动修复
func (c *Checker) reconcileOne(ctx context.Context, order *OrderRow, bill *C2BBillRow, res *CheckResult) {
	res.Total++

	// 有c2b转账单但订单不存在: 无修复规则, 人工介入
	if order == nil {
		res.Inconsistent++
		res.add(Inconsistency{
			Severity: SeverityWarn,
			Kind:     "c2b_bill_no_order",
			Key:      bill.TransactionId,
			Detail:   fmt.Sprintf("c2b转账单存在但订单不存在, bill_state=%d", bill.State),
		})
		return
	}

	// 参与核对的订单按交易状态计数(写入 t_check_record 的字段1~4)
	res.OrdersTotal++
	switch order.TradeState {
	case orderTradeStateSuccess:
		res.OrdersSuccess++
	case orderTradeStateClose:
		res.OrdersClosed++
	case orderTradeStateInit:
		res.OrdersInit++
	}

	// c2b转账单不存在
	if bill == nil {
		switch order.TradeState {
		case orderTradeStateSuccess:
			// 订单成功但无c2b转账单: 无修复规则, 人工介入
			res.Inconsistent++
			res.add(Inconsistency{
				Severity: SeverityWarn,
				Kind:     "order_success_no_c2b_bill",
				Key:      order.TransactionId,
				Detail:   "订单成功但c2b转账单不存在",
			})
		case orderTradeStateInit:
			// 订单待支付且无c2b转账单: 支付未发起, 不处理
			res.Skipped++
		default:
			// 订单已关且无c2b转账单: 正常(未支付即关单)
			res.Consistent++
		}
		return
	}

	issue, ok := c.compareState(order, bill)
	if !ok {
		res.Consistent++
		return
	}

	// 状态单边不一致, 按规则修复
	if c.cfg.EnableRepair {
		issue.Repaired = c.repair(ctx, order, bill, issue.Kind)
	}
	if issue.Repaired {
		res.Repaired++
		switch issue.Kind {
		case "order_init_bill_success", "order_closed_bill_success":
			// 订单被补为成功: trade_itg补单 / 主库补成功
			res.RepairSupply++
		case "order_init_bill_closed", "order_success_bill_closed":
			// 订单被关闭: trade_itg关单 / 主库关单
			res.RepairClose++
		}
	}
	res.Inconsistent++
	res.add(issue)
}

// compareState 状态比对; 返回不一致明细, ok=false 表示一致
func (c *Checker) compareState(order *OrderRow, bill *C2BBillRow) (Inconsistency, bool) {
	switch {
	case order.TradeState == orderTradeStateInit && bill.State == c2bBillStateSuccess:
		// c2b转账单已成功但订单还是初始状态: 调 trade_itg 纯补单接口补成功
		return Inconsistency{
			Severity: SeverityCritical,
			Kind:     "order_init_bill_success",
			Key:      order.TransactionId,
			Detail: fmt.Sprintf("c2b转账单已成功(pay_time=%s)但订单为待支付状态, 应调trade_itg补单接口将订单补成功",
				bill.PayTime.Format("2006-01-02 15:04:05")),
		}, true
	case order.TradeState == orderTradeStateInit && bill.State == c2bBillStateClose:
		// c2b转账单已关单但订单还是初始状态: 调 trade_itg 纯关单接口关闭订单
		return Inconsistency{
			Severity: SeverityCritical,
			Kind:     "order_init_bill_closed",
			Key:      order.TransactionId,
			Detail:   "c2b转账单已关闭但订单为待支付状态, 应调trade_itg关单接口关闭订单",
		}, true
	case order.TradeState == orderTradeStateSuccess && bill.State == c2bBillStateClose:
		// c2b转账单已关但订单未关(成功): 需要关订单
		return Inconsistency{
			Severity: SeverityCritical,
			Kind:     "order_success_bill_closed",
			Key:      order.TransactionId,
			Detail:   "c2b转账单已关闭但订单为成功状态, 应关闭订单",
		}, true
	case order.TradeState == orderTradeStateClose && bill.State == c2bBillStateSuccess:
		// c2b转账单已成功但订单未成功(已关): 需要补订单成功
		return Inconsistency{
			Severity: SeverityCritical,
			Kind:     "order_closed_bill_success",
			Key:      order.TransactionId,
			Detail: fmt.Sprintf("c2b转账单已成功(pay_time=%s)但订单已关闭, 应将订单补成功",
				bill.PayTime.Format("2006-01-02 15:04:05")),
		}, true
	}
	// 订单成功+c2b成功 / 订单已关+c2b已关: 一致
	return Inconsistency{}, false
}

// repair 按不一致类型修复订单状态
// 待支付(初始状态)订单: 走 trade_itg 纯补单/纯关单接口(带订单与c2b转账单一致性校验)
// 终态订单(成功/已关): trade_itg 接口拒绝处理终态订单, 走主库条件更新, 并发变更时不误改
func (c *Checker) repair(ctx context.Context, order *OrderRow, bill *C2BBillRow, kind string) bool {
	switch kind {
	case "order_init_bill_success":
		// c2b已成功 + 订单待支付 → 调 trade_itg 纯补单接口, 将订单补成功
		return c.repairBySupplyOrder(ctx, order)
	case "order_init_bill_closed":
		// c2b已关单 + 订单待支付 → 调 trade_itg 纯关单接口, 将订单关闭
		return c.repairByCloseOrder(ctx, order)
	case "order_success_bill_closed", "order_closed_bill_success":
		return c.repairByMasterDB(ctx, order, bill, kind)
	}
	return false
}

// repairBySupplyOrder 调 trade_itg SupplyOrder 补单: 入参带订单关键数据, 由服务端与c2b转账单做一致性校验
func (c *Checker) repairBySupplyOrder(ctx context.Context, order *OrderRow) bool {
	if c.tradeItg == nil {
		return false
	}
	_, err := c.tradeItg.SupplyOrder(ctx, &trade_itg_pb.SupplyOrderReq{
		TransactionId: order.TransactionId,
		OutOrderNo:    order.OutOrderNo,
		MerchantId:    order.MerchantId,
		UserId:        order.UserId,
		Amount:        order.Amount,
	})
	if err != nil {
		c.logger.Errorf("repair by TradeItg.SupplyOrder failed, transaction_id: %s, err: %v", order.TransactionId, err)
		return false
	}
	return true
}

// repairByCloseOrder 调 trade_itg CloseOrder 关单: 服务端确认c2b转账单已关后才关订单
func (c *Checker) repairByCloseOrder(ctx context.Context, order *OrderRow) bool {
	if c.tradeItg == nil {
		return false
	}
	_, err := c.tradeItg.CloseOrder(ctx, &trade_itg_pb.CloseOrderReq{
		TransactionId: order.TransactionId,
	})
	if err != nil {
		c.logger.Errorf("repair by TradeItg.CloseOrder failed, transaction_id: %s, err: %v", order.TransactionId, err)
		return false
	}
	return true
}

// repairByMasterDB 终态订单修复: 主库条件更新(trade_itg 接口拒绝处理终态订单)
func (c *Checker) repairByMasterDB(ctx context.Context, order *OrderRow, bill *C2BBillRow, kind string) bool {
	if c.orderMaster == nil {
		return false
	}

	var (
		repaired bool
		err      error
	)
	switch kind {
	case "order_success_bill_closed":
		// c2b已关 + 订单成功 → 关闭订单
		repaired, err = c.orderMaster.CloseSuccessOrder(ctx, order.TransactionId)
	case "order_closed_bill_success":
		// c2b已成功 + 订单已关 → 补订单成功, 支付时间以c2b转账单为准
		repaired, err = c.orderMaster.SupplyClosedOrderSuccess(ctx, order.TransactionId, bill.PayTime)
	}
	if err != nil {
		c.logger.Errorf("repair order failed, transaction_id: %s, kind: %s, err: %v", order.TransactionId, kind, err)
		return false
	}
	if !repaired {
		// 条件未命中: 订单状态已被其他链路变更(如正常收单流程), 视为已收敛不算失败
		c.logger.Infof("repair skipped, order state changed, transaction_id: %s, kind: %s", order.TransactionId, kind)
	}
	return repaired
}

func chunk[T any](items []T, size int64) [][]T {
	if len(items) == 0 {
		return nil
	}
	if size < 1 {
		size = 1
	}
	batches := make([][]T, 0, (int64(len(items))+size-1)/size)
	for start := 0; start < len(items); start += int(size) {
		end := start + int(size)
		if end > len(items) {
			end = len(items)
		}
		batches = append(batches, items[start:end])
	}
	return batches
}
