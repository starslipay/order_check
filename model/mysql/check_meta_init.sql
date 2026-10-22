-- 核对任务基本信息表初始化脚本(核对任务字段名与含义定义)
-- 表放在 order_db 中, 与订单表同库

-- Use database
USE `order_db`;

DROP TABLE IF EXISTS `t_check_meta`;
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='核对任务基本信息表(字段名与含义定义)';

insert into `t_check_meta` (`check_id`, `check_name`, `field1_name`, `field2_name`, `field3_name`, `field4_name`, `field5_name`, `field6_name`, `field7_name`, `field8_name`, `field9_name`, `field10_name`)
values (4559001, 'order_check_minute', 'order_total', 'order_success', 'order_closed', 'order_init', 'repair_supply', 'repair_close', 'check_start_time', 'check_end_time', 'data_start_time', 'data_end_time');

insert into `t_check_meta` (`check_id`, `check_name`, `field1_name`, `field2_name`, `field3_name`, `field4_name`, `field5_name`, `field6_name`, `field7_name`, `field8_name`, `field9_name`, `field10_name`)
values (4559002, 'order_check_hour', 'order_total', 'order_success', 'order_closed', 'order_init', 'repair_supply', 'repair_close', 'check_start_time', 'check_end_time', 'data_start_time', 'data_end_time');

insert into `t_check_meta` (`check_id`, `check_name`, `field1_name`, `field2_name`, `field3_name`, `field4_name`, `field5_name`, `field6_name`, `field7_name`, `field8_name`, `field9_name`, `field10_name`)
values (4559003, 'order_check_day', 'order_total', 'order_success', 'order_closed', 'order_init', 'repair_supply', 'repair_close', 'check_start_time', 'check_end_time', 'data_start_time', 'data_end_time');

-- linux:  mysql -h 127.0.0.1 -P 3306 -u root -proot123456 < check_meta_init.sql
-- windows: Get-Content -Encoding UTF8 check_meta_init.sql | mysql -h 127.0.0.1 -P 3306 -u root -proot123456
-- docker: docker exec -i mysql-master mysql -uroot -proot123456 < check_meta_init.sql
