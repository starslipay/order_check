-- 核对记录表: 只记录每轮核对的字段值
-- 字段名称定义在 t_check_meta 中(按 check_id 关联), 本表只存值
-- field1_data ~ field6_data 为数字型, field7_data ~ field10_data 为字符串型
CREATE TABLE `t_check_record` (
  `record_id` BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `check_id` BIGINT NOT NULL COMMENT '核对任务ID(关联 t_check_meta.check_id)',
  `check_name` VARCHAR(64) NOT NULL COMMENT '核对名称(关联 t_check_meta.check_name)',
  `field1_data` BIGINT NOT NULL DEFAULT 0 COMMENT '字段1数据(数字)',
  `field2_data` BIGINT NOT NULL DEFAULT 0 COMMENT '字段2数据(数字)',
  `field3_data` BIGINT NOT NULL DEFAULT 0 COMMENT '字段3数据(数字)',
  `field4_data` BIGINT NOT NULL DEFAULT 0 COMMENT '字段4数据(数字)',
  `field5_data` BIGINT NOT NULL DEFAULT 0 COMMENT '字段5数据(数字)',
  `field6_data` BIGINT NOT NULL DEFAULT 0 COMMENT '字段6数据(数字)',
  `field7_data` VARCHAR(256) NOT NULL DEFAULT '' COMMENT '字段7数据(字符串)',
  `field8_data` VARCHAR(256) NOT NULL DEFAULT '' COMMENT '字段8数据(字符串)',
  `field9_data` VARCHAR(256) NOT NULL DEFAULT '' COMMENT '字段9数据(字符串)',
  `field10_data` VARCHAR(256) NOT NULL DEFAULT '' COMMENT '字段10数据(字符串)',
  `create_time` datetime DEFAULT CURRENT_TIMESTAMP,
  `update_time` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`record_id`),
  INDEX `idx_check_id_create_time` (`check_id`, `create_time`),
  INDEX `idx_create_time` (`create_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='核对记录表(字段值)';

-- goctl model mysql ddl -src check_record.sql -dir .
-- -c：开启缓存（redis，可选，不加则无缓存）
