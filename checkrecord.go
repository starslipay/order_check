package main

import (
	"context"
	"time"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// recordTimeLayout 核对记录中时间字段的格式(字段7~10为字符串型)
const recordTimeLayout = "2006-01-02 15:04:05"

// CheckRecord t_check_record 核对记录, 字段值的含义由 t_check_meta 定义
type CheckRecord struct {
	RecordId    int64  `db:"record_id"`    // 主键ID
	CheckId     int64  `db:"check_id"`     // 核对任务ID(t_check_meta.check_id)
	CheckName   string `db:"check_name"`   // 核对名称
	Field1Data  int64  `db:"field1_data"`  // 字段1数据(数字)
	Field2Data  int64  `db:"field2_data"`  // 字段2数据(数字)
	Field3Data  int64  `db:"field3_data"`  // 字段3数据(数字)
	Field4Data  int64  `db:"field4_data"`  // 字段4数据(数字)
	Field5Data  int64  `db:"field5_data"`  // 字段5数据(数字)
	Field6Data  int64  `db:"field6_data"`  // 字段6数据(数字)
	Field7Data  string `db:"field7_data"`  // 字段7数据(字符串)
	Field8Data  string `db:"field8_data"`  // 字段8数据(字符串)
	Field9Data  string `db:"field9_data"`  // 字段9数据(字符串)
	Field10Data string `db:"field10_data"` // 字段10数据(字符串)
}

type checkRecordModel struct {
	conn sqlx.SqlConn
}

func newCheckRecordModel(conn sqlx.SqlConn) *checkRecordModel {
	return &checkRecordModel{conn: conn}
}

// Insert 写入一条核对记录
// t_check_record 每轮核对插一行, 按 check_id + create_time 可查历史批次
func (m *checkRecordModel) Insert(ctx context.Context, r *CheckRecord) error {
	_, err := m.conn.ExecCtx(ctx, `
		insert into t_check_record
			(check_id, check_name,
			 field1_data, field2_data, field3_data, field4_data, field5_data, field6_data,
			 field7_data, field8_data, field9_data, field10_data)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.CheckId, r.CheckName,
		r.Field1Data, r.Field2Data, r.Field3Data, r.Field4Data, r.Field5Data, r.Field6Data,
		r.Field7Data, r.Field8Data, r.Field9Data, r.Field10Data)
	return err
}

// recordTime 时间字段转字符串, 零值写空串
func recordTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(recordTimeLayout)
}
