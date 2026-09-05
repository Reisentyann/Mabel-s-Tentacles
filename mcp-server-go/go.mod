module github.com/Reisentyann/Mabel-s-Tentacles/mcp-server-go

go 1.26.5

require (
	github.com/Reisentyann/Mabel-s-Tentacles/describer-go v0.0.0
	github.com/Reisentyann/Mabel-s-Tentacles/indexer-go v0.0.0
	github.com/Reisentyann/Mabel-s-Tentacles/manager-go v0.0.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/jackc/pgx/v5 v5.10.0
	github.com/mark3labs/mcp-go v0.58.0
	golang.org/x/crypto v0.55.0
	gopkg.in/yaml.v3 v3.0.1
)

// 同仓库同级模块（Docker 构建上下文必须为仓库根才能拿到它们）
replace github.com/Reisentyann/Mabel-s-Tentacles/describer-go => ../describer-go

replace github.com/Reisentyann/Mabel-s-Tentacles/indexer-go => ../indexer-go

replace github.com/Reisentyann/Mabel-s-Tentacles/manager-go => ../manager-go

require (
	github.com/google/jsonschema-go v0.4.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/rwcarlsen/goexif v0.0.0-20190401172101-9e8deecbddbd // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/image v0.45.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)
