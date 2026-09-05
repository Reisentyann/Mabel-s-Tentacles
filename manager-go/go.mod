module github.com/Reisentyann/Mabel-s-Tentacles/manager-go

go 1.26.5

require github.com/Reisentyann/Mabel-s-Tentacles/describer-go v0.0.0

require (
	github.com/rwcarlsen/goexif v0.0.0-20190401172101-9e8deecbddbd // indirect
	golang.org/x/image v0.45.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)

// 同仓库同级模块（updater 用描述引擎；Docker 构建上下文 = 仓库根才能拿到）
replace github.com/Reisentyann/Mabel-s-Tentacles/describer-go => ../describer-go
