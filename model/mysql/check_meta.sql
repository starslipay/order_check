-- 核对任务基本信息表: 定义每个核对任务的字段名称
-- 每个核对任务一行, check_name 唯一; 字段1~6为数字型, 字段7~10为字符串型
-- 字段名的含义由使用方(核对任务)自行维护, 初始化数据见 check_meta_init.sql
CREATE TABLE `t_check_meta` (
  `check_id` BIGINT NOT NULL COMMENT '主键ID',
  `check_name` VARCHAR(64) NOT NULL COMMENT '核对名称(如 order_check_minute)',
  `field1_name` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '字段1名称(数字)',
  `field2_name` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '字段2名称(数字)',
  `field3_name` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '字段3名称(数字)',
  `field4_name` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '字段4名称(数字)',
  `field5_name` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '字段5名称(数字)',
  `field6_name` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '字段6名称(数字)',
  `field7_name` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '字段7名称(字符串)',
  `field8_name` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '字段8名称(字符串)',
  `field9_name` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '字段9名称(字符串)',
  `field10_name` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '字段10名称(字符串)',
  `create_time` datetime DEFAULT CURRENT_TIMESTAMP,
  `update_time` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`check_id`),
  UNIQUE KEY `uk_check_name` (`check_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='核对任务基本信息表(字段名定义)';

-- goctl model mysql ddl -src check_meta.sql -dir .
-- -c：开启缓存（redis，可选，不加则无缓存）
