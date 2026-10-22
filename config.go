package main

import (
	"github.com/zeromicro/go-zero/core/stores/sqlx"
	"github.com/zeromicro/go-zero/zrpc"
)

// Config 订单与c2b转账单核对任务配置
type Config struct {
	// OrderDB 订单库(order_db.t_order), 建议指向从库, 避免核对流量压主库
	OrderDB sqlx.SqlConf
	// OrderMasterDB 订单库主库, 仅 EnableRepair=true 时用于修复终态订单(成功/已关)的状态写回
	OrderMasterDB sqlx.SqlConf `json:",optional"`
	// AccountDB 账户库(account_db.t_c2b_bill), c2b转账单所在库, 建议指向从库
	AccountDB sqlx.SqlConf

	// TradeItgRpcConfig trade_itg 服务客户端, 仅 EnableRepair=true 时使用
	// 待支付(初始状态)订单的调账走 trade_itg 纯补单(SupplyOrder)/纯关单(CloseOrder)接口
	TradeItgRpcConfig zrpc.RpcClientConf `json:",optional"`

	// CheckInterval 核对周期(秒)
	CheckInterval int64 `json:",default=60"`
	// TimeStart/TimeEnd 增量核对的时间窗口(相对当前时间的秒偏移)
	// TimeEnd 留负数余量(-30): 刚成功的订单其c2b单可能还未同步到从库, 避免误判
	TimeStart int64 `json:",default=-300"`
	TimeEnd   int64 `json:",default=-30"`

	// IsFullScan 全量核对(按主键游标分页扫描整表), 低频巡检开启
	IsFullScan bool `json:",default=false"`
	// EnableRepair 发现不一致时是否自动修复订单状态; false 只告警
	EnableRepair bool `json:",default=false"`

	// PageSize 每次 DB 分页/每批 IN 查询的记录数
	PageSize int64 `json:",default=1000"`

	// CheckRecord 核对记录写入配置: 每轮核对结束后写一条 t_check_record, 字段含义见 t_check_meta
	CheckRecord CheckRecordConf `json:",optional"`
}

// CheckRecordConf 核对记录写入配置, 与 t_check_meta 中的登记信息对应
type CheckRecordConf struct {
	// CheckId 核对任务ID, 对应 t_check_meta.check_id; 为 0 时不写入核对记录
	CheckId int64 `json:",default=0"`
	// CheckName 核对名称, 对应 t_check_meta.check_name
	CheckName string `json:",optional"`
}
