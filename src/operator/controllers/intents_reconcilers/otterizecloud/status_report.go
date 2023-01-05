package otterizecloud

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func PeriodicallyReportConnectionToCloud(client CloudClient) {
	interval := viper.GetInt(otterizecloudclient.ComponentReportIntervalKey)
	go func() {
		runPeriodicReportConnection(interval, client)
	}()
}

func runPeriodicReportConnection(interval int, client CloudClient) {
	ctx, cloudClientCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cloudClientCancel()
	cloudUploadTicker := time.NewTicker(time.Second * time.Duration(interval))

	logrus.Info("Starting cloud connection ticker")
	for {
		select {
		case <-cloudUploadTicker.C:
			client.ReportComponentStatus(ctx, graphqlclient.ComponentTypeIntentsOperator)

		case <-ctx.Done():
			logrus.Info("Periodic upload exit")
			return
		}
	}
}
