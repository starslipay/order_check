package main

import (
	"context"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// 订单交易状态, 与 order_mgr/internal/consts 保持一致
const (
	orderTradeStateInit    = 1  // 初始状态(待支付)
	orderTradeStateSuccess = 2  // 成功
	orderTradeStateClose   = 99 // 关闭
)

// c2b转账单状态, 与 account_mgr/internal/consts 保持一致
const (
	c2bBillStateSuccess = 1  // 成功
	c2bBillStateClose   = 99 // 关闭
)

// OrderRow 订单表 t_order(order_db) 参与核对的字段
type OrderRow struct {
	TransactionId string    `db:"transaction_id"` // 交易ID
	OutOrderNo    string    `db:"out_order_no"`   // 商户订单号(调trade_itg补单接口需要)
	MerchantId    string    `db:"merchant_id"`    // 商户ID(调trade_itg补单接口需要)
	UserId        string    `db:"user_id"`        // 用户ID(调trade_itg补单接口需要)
	Amount        int64     `db:"amount"`         // 金额(调trade_itg补单接口需要)
	TradeState    int64     `db:"trade_state"`    // 交易状态: 1-待支付 2-成功 99-关闭
	PayTime       time.Time `db:"pay_time"`       // 支付时间
	UpdateTime    time.Time `db:"update_time"`    // 更新时间
}

// C2BBillRow c2b转账单表 t_c2b_bill(account_db) 参与核对的字段
type C2BBillRow struct {
	TransactionId string    `db:"transaction_id"` // 交易ID
	State         int64     `db:"state"`          // 单状态: 1-成功 99-关闭
	PayTime       time.Time `db:"pay_time"`       // 支付时间
	UpdateTime    time.Time `db:"update_time"`    // 更新时间
}

const orderColumns = "transaction_id, out_order_no, merchant_id, user_id, amount, trade_state, pay_time, update_time"
const c2bBillColumns = "transaction_id, state, pay_time, update_time"

type orderModel struct {
	conn sqlx.SqlConn
}

func newOrderModel(conn sqlx.SqlConn) *orderModel {
	return &orderModel{conn: conn}
}

// FindByWindow 增量核对: 查窗口内发生过变更的订单, 走 idx_update_time 索引
func (m *orderModel) FindByWindow(ctx context.Context, start, end time.Time) ([]*OrderRow, error) {
	query := "select " + orderColumns + " from t_order where update_time >= ? and update_time < ?"
	var rows []*OrderRow
	err := m.conn.QueryRowsCtx(ctx, &rows, query, start, end)
	return rows, err
}

// FindByCursor 全量核对: 按主键 transaction_id 游标翻页, 避免 OFFSET 深翻页
func (m *orderModel) FindByCursor(ctx context.Context, afterId string, limit int64) ([]*OrderRow, error) {
	query := "select " + orderColumns + " from t_order where transaction_id > ? order by transaction_id asc limit ?"
	var rows []*OrderRow
	err := m.conn.QueryRowsCtx(ctx, &rows, query, afterId, limit)
	return rows, err
}

// FindByIds 按交易ID批量查订单, 返回 transaction_id -> 订单 的映射
func (m *orderModel) FindByIds(ctx context.Context, ids []string) (map[string]*OrderRow, error) {
	result := make(map[string]*OrderRow, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	query := "select " + orderColumns + " from t_order where transaction_id in (" + placeholders(len(ids)) + ")"
	var rows []*OrderRow
	if err := m.conn.QueryRowsCtx(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	for _, r := range rows {
		result[r.TransactionId] = r
	}
	return result, nil
}

// CloseSuccessOrder 修复: 将"成功"状态的订单关单
// 条件更新 trade_state=2, 并发下订单状态已被其他链路变更时不误改; 返回是否实际修改成功
func (m *orderModel) CloseSuccessOrder(ctx context.Context, transactionId string) (bool, error) {
	ret, err := m.conn.ExecCtx(ctx,
		"update t_order set trade_state = ? where transaction_id = ? and trade_state = ?",
		orderTradeStateClose, transactionId, orderTradeStateSuccess)
	if err != nil {
		return false, err
	}
	rows, err := ret.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

// SupplyClosedOrderSuccess 修复: 将"已关闭"状态的订单补为成功, 支付时间以c2b转账单为准
// 条件更新 trade_state=99, 并发下订单状态已被其他链路变更时不误改; 返回是否实际修改成功
func (m *orderModel) SupplyClosedOrderSuccess(ctx context.Context, transactionId string, payTime time.Time) (bool, error) {
	ret, err := m.conn.ExecCtx(ctx,
		"update t_order set trade_state = ?, pay_time = ? where transaction_id = ? and trade_state = ?",
		orderTradeStateSuccess, payTime, transactionId, orderTradeStateClose)
	if err != nil {
		return false, err
	}
	rows, err := ret.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

type c2bBillModel struct {
	conn sqlx.SqlConn
}

func newC2BBillModel(conn sqlx.SqlConn) *c2bBillModel {
	return &c2bBillModel{conn: conn}
}

// FindByWindow 增量核对: 查窗口内发生过变更的c2b转账单, 走 idx_update_time 索引
func (m *c2bBillModel) FindByWindow(ctx context.Context, start, end time.Time) ([]*C2BBillRow, error) {
	query := "select " + c2bBillColumns + " from t_c2b_bill where update_time >= ? and update_time < ?"
	var rows []*C2BBillRow
	err := m.conn.QueryRowsCtx(ctx, &rows, query, start, end)
	return rows, err
}

// FindByCursor 全量核对: 按主键 transaction_id 游标翻页
func (m *c2bBillModel) FindByCursor(ctx context.Context, afterId string, limit int64) ([]*C2BBillRow, error) {
	query := "select " + c2bBillColumns + " from t_c2b_bill where transaction_id > ? order by transaction_id asc limit ?"
	var rows []*C2BBillRow
	err := m.conn.QueryRowsCtx(ctx, &rows, query, afterId, limit)
	return rows, err
}

// FindByIds 按交易ID批量查c2b转账单, 返回 transaction_id -> 转账单 的映射
func (m *c2bBillModel) FindByIds(ctx context.Context, ids []string) (map[string]*C2BBillRow, error) {
	result := make(map[string]*C2BBillRow, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	query := "select " + c2bBillColumns + " from t_c2b_bill where transaction_id in (" + placeholders(len(ids)) + ")"
	var rows []*C2BBillRow
	if err := m.conn.QueryRowsCtx(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	for _, r := range rows {
		result[r.TransactionId] = r
	}
	return result, nil
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
