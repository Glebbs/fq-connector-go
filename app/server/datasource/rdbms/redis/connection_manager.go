package redis

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/ydb-platform/fq-connector-go/app/config"
	rdbms_utils "github.com/ydb-platform/fq-connector-go/app/server/datasource/rdbms/utils"
	"github.com/ydb-platform/fq-connector-go/common"
)

var _ rdbms_utils.ConnectionManager = (*connectionManager)(nil)

type connectionManager struct {
	rdbms_utils.ConnectionManagerBase
	cfg *config.TMsSQLServerConfig
}

func (c *connectionManager) Make(
	params *rdbms_utils.ConnectionManagerMakeParams,
) ([]rdbms_utils.Connection, error) {
	dsi, ctx, logger := params.DataSourceInstance, params.Ctx, params.Logger

	options := &redis.Options{
		Addr: fmt.Sprintf("%s:%d",
			dsi.GetEndpoint().GetHost(),
			dsi.GetEndpoint().GetPort()),
		Password: dsi.Credentials.GetBasic().GetPassword(),
		DB:       0,
	}

	if dsi.UseTls {
		options.TLSConfig = &tls.Config{
			InsecureSkipVerify: false,
		}
	}

	db := redis.NewClient(options)

	pingCtx, pingCtxCancel := context.WithTimeout(ctx, common.MustDurationFromString(c.cfg.PingConnectionTimeout))
	defer pingCtxCancel()

	if _, err := db.Ping(pingCtx).Result(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis with TLS: %v", err)
	}

	queryLogger := c.QueryLoggerFactory.Make(logger)

	return []rdbms_utils.Connection{&Connection{db, queryLogger}}, nil
}

func (*connectionManager) Release(_ context.Context, logger *zap.Logger, cs []rdbms_utils.Connection) {
	for _, conn := range cs {
		common.LogCloserError(logger, conn, "close connection")
	}
}

func NewConnectionManager(
	cfg *config.TMsSQLServerConfig,
	base rdbms_utils.ConnectionManagerBase) rdbms_utils.ConnectionManager {
	return &connectionManager{ConnectionManagerBase: base, cfg: cfg}
}
