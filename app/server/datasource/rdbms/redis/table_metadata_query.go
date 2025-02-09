package redis

import (
	"fmt"

	_ "github.com/denisenkom/go-mssqldb"

	api_service_protos "github.com/ydb-platform/fq-connector-go/api/service/protos"
	rdbms_utils "github.com/ydb-platform/fq-connector-go/app/server/datasource/rdbms/utils"
)

func TableMetadataQuery(request *api_service_protos.TDescribeTableRequest) (string, *rdbms_utils.QueryArgs) {
	query := "DESCRIBE example_table"

	var args rdbms_utils.QueryArgs

	args.AddUntyped(request.Table)
	fmt.Println("\n================== TableMetadataQuery ==================")
	return query, &args
}
