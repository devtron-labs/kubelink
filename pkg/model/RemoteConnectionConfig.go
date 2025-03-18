package model

import (
	remoteConnectionBean "github.com/devtron-labs/common-lib/utils/remoteConnection/bean"
)

type RemoteConnectionConfig struct {
	tableName        struct{}                                    `sql:"remote_connection_config" pg:",discard_unknown_columns"`
	Id               int                                         `sql:"id,pk"`
	ConnectionMethod remoteConnectionBean.RemoteConnectionMethod `sql:"connection_method"`
	ProxyUrl         string                                      `sql:"proxy_url"`
	SSHServerAddress string                                      `sql:"ssh_server_address"`
	SSHUsername      string                                      `sql:"ssh_username"`
	SSHPassword      string                                      `sql:"ssh_password"`
	SSHAuthKey       string                                      `sql:"ssh_auth_key"`
	Deleted          bool                                        `sql:"deleted,notnull"`
	AuditLog
}
