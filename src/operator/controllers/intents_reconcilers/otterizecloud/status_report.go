package otterizecloud

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"time"
)

func StartPeriodicallyReportConnectionToCloud(client CloudClient, ctx context.Context) {
	interval := viper.GetInt(otterizecloudclient.ComponentReportIntervalKey)
	go func() {
		runPeriodicReportConnection(interval, client, ctx)
	}()
}

func runPeriodicReportConnection(interval int, client CloudClient, ctx context.Context) {
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
