module github.com/Reisentyann/Mabel-s-Tentacles/describer-go

go 1.26.5

require (
	github.com/rwcarlsen/goexif v0.0.0-20190401172101-9e8deecbddbd
	golang.org/x/image v0.45.0
	golang.org/x/text v0.41.0
)

// cmd/verify 用共享盘读助手（common 为零业务依赖的叶子模块，不破坏引擎纯库立场）
require github.com/Reisentyann/Mabel-s-Tentacles/common v0.0.0

// 同仓库同级模块（Docker 构建上下文 = 仓库根才能拿到）
replace github.com/Reisentyann/Mabel-s-Tentacles/common => ../common
