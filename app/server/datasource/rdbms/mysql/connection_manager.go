package mysql

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/go-mysql-org/go-mysql/client"
	pingcap_errors "github.com/pingcap/errors"
	"go.uber.org/zap"

	"github.com/ydb-platform/fq-connector-go/app/config"
	rdbms_utils "github.com/ydb-platform/fq-connector-go/app/server/datasource/rdbms/utils"
	"github.com/ydb-platform/fq-connector-go/common"
)

var _ rdbms_utils.ConnectionManager = (*connectionManager)(nil)

type connectionManager struct {
	rdbms_utils.ConnectionManagerBase
	cfg *config.TMySQLConfig
	// TODO: cache of connections, remove unused connections with TTL
}

func (c *connectionManager) Make(
	params *rdbms_utils.ConnectionParams,
) ([]rdbms_utils.Connection, error) {
	dsi, ctx, logger := params.DataSourceInstance, params.Ctx, params.Logger
	optionFuncs := make([]func(c *client.Conn), 0)

	if dsi.GetCredentials().GetBasic() == nil {
		return nil, fmt.Errorf("currently only basic auth is supported")
	}

	if dsi.GetUseTls() {
		optionFuncs = append(optionFuncs, func(c *client.Conn) { c.UseSSL(true) })
	}

	queryLogger := c.QueryLoggerFactory.Make(logger)

	endpoint := dsi.GetEndpoint()
	addr := fmt.Sprintf("%s:%d", endpoint.GetHost(), endpoint.GetPort())

	db := dsi.GetDatabase()

	creds := dsi.GetCredentials().GetBasic()
	user := creds.GetUsername()
	password := creds.GetPassword()

	// TODO: support cert-based auth

	dialer := &net.Dialer{
		Timeout: common.MustDurationFromString(c.cfg.OpenConnectionTimeout),
	}
	proto := "tcp"

	if strings.Contains(addr, "/") {
		return nil, errors.New("unix socket connections are unsupported")
	}

	openConnectionCtx, openConnectionCtxCancel := context.WithTimeout(ctx, common.MustDurationFromString(c.cfg.OpenConnectionTimeout))
	defer openConnectionCtxCancel()

	conn, err := client.ConnectWithDialer(
		openConnectionCtx,
		proto,
		addr,
		user,
		password,
		db,
		dialer.DialContext,
		optionFuncs...)
	if err != nil {
		return nil, fmt.Errorf("connect with dialer: %w", pingcap_errors.Cause(err))
	}

	// YQ-3608: force using UTC for date/time formats were possible
	_, err = conn.Execute("SET time_zone = 'UTC'")
	if err != nil {
		return nil, fmt.Errorf("set time zone: %w", err)
	}

	return []rdbms_utils.Connection{&connection{queryLogger, conn, c.cfg, dsi, params.TableName}}, nil
}

func (*connectionManager) Release(_ context.Context, logger *zap.Logger, cs []rdbms_utils.Connection) {
	for _, cs := range cs {
		common.LogCloserError(logger, cs, "close connection")
	}
}

func NewConnectionManager(cfg *config.TMySQLConfig, base rdbms_utils.ConnectionManagerBase) rdbms_utils.ConnectionManager {
	return &connectionManager{ConnectionManagerBase: base, cfg: cfg}
}
