package operator_cloud_client

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/otterize/intents-operator/src/shared/telemetries/errorreporter"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"time"
)

func StartPeriodicallyReportConnectionToCloud(client CloudClient, ctx context.Context) {
	interval := viper.GetInt(otterizecloudclient.ComponentReportIntervalKey)
	go func() {
		defer errorreporter.AutoNotify()
		runPeriodicReportConnection(interval, client, ctx)
	}()
}

func runPeriodicReportConnection(interval int, client CloudClient, ctx context.Context) {
	cloudUploadTicker := time.NewTicker(time.Second * time.Duration(interval))

	logrus.Info("Starting cloud connection ticker")
	reportStatus(ctx, client)

	for {
		select {
		case <-cloudUploadTicker.C:
			reportStatus(ctx, client)

		case <-ctx.Done():
			logrus.Info("Periodic upload exit")
			return
		}
	}
}

func reportStatus(ctx context.Context, client CloudClient) {
	timeoutCtx, cancel := context.WithTimeout(ctx, viper.GetDuration(otterizecloudclient.CloudClientTimeoutKey))
	defer cancel()

	client.ReportComponentStatus(timeoutCtx, graphqlclient.ComponentTypeIntentsOperator)
}
