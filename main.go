package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/starslipay/trade_itg/trade_itg_pb"
	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
	"github.com/zeromicro/go-zero/zrpc"
)

var configFile = flag.String("f", "etc/config_minute.yaml", "the config file")

func main() {
	flag.Parse()

	var c Config
	conf.MustLoad(*configFile, &c)

	logx.MustSetup(logx.LogConf{
		ServiceName: "order_check",
		Mode:        "console",
	})

	orderConn := sqlx.MustNewConn(c.OrderDB)
	accountConn := sqlx.MustNewConn(c.AccountDB)

	// 修复要写订单主库(终态订单)/调 trade_itg(待支付订单), 核对记录也要写主库
	// 只读核对且不写核对记录时不需要这些连接
	var orderMasterConn sqlx.SqlConn
	var tradeItgClient trade_itg_pb.TradeItgClient
	var checkRecord *checkRecordModel
	if c.CheckRecord.CheckId > 0 || c.EnableRepair {
		orderMasterConn = sqlx.MustNewConn(c.OrderMasterDB)
	}
	if c.EnableRepair {
		tradeItgClient = trade_itg_pb.NewTradeItgClient(zrpc.MustNewClient(c.TradeItgRpcConfig).Conn())
	}
	if c.CheckRecord.CheckId > 0 {
		checkRecord = newCheckRecordModel(orderMasterConn)
	}

	checker := NewChecker(c, orderConn, orderMasterConn, accountConn, tradeItgClient, checkRecord)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Printf("Starting order checker...\n")
	fmt.Printf("Config file:      %s\n", *configFile)
	fmt.Printf("Check interval:   %d seconds\n", c.CheckInterval)
	if c.IsFullScan {
		fmt.Printf("Mode:             full scan, page size %d\n", c.PageSize)
	} else {
		fmt.Printf("Mode:             window, %d ~ %d seconds\n", c.TimeStart, c.TimeEnd)
	}
	fmt.Printf("Enable repair:    %v\n", c.EnableRepair)
	if c.CheckRecord.CheckId > 0 {
		fmt.Printf("Check record:     check_id %d, check_name %s\n", c.CheckRecord.CheckId, c.CheckRecord.CheckName)
	} else {
		fmt.Printf("Check record:     disabled\n")
	}

	go func() {
		ticker := time.NewTicker(time.Duration(c.CheckInterval) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				runCheck(ctx, checker)
			case <-ctx.Done():
				return
			}
		}
	}()

	// 启动先跑一轮, 之后按 CheckInterval 周期执行
	runCheck(ctx, checker)

	<-sigChan
	fmt.Println("Shutting down...")
	cancel()
}

func runCheck(ctx context.Context, checker *Checker) {
	batchStart := time.Now()
	fmt.Printf("\n[%s] Starting check batch...\n", batchStart.Format("2006-01-02 15:04:05"))

	result := checker.Run(ctx)
	fmt.Println(result.String())
	fmt.Printf("[%s] Check batch finished, cost %v\n",
		time.Now().Format("2006-01-02 15:04:05"), time.Since(batchStart))
}
