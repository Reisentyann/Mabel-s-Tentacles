@echo off
chcp 65001 >nul
echo Starting Agent Project...

:: Check if docker is running
docker info >nul 2>&1
if %errorlevel% neq 0 (
    echo Docker is not running. Please start Docker Desktop and try again.
    pause
    exit /b 1
)

:: Remove any existing old containers with the same names to prevent conflicts
echo Cleaning up existing containers...
docker-compose down >nul 2>&1
docker rm -f agent_postgres agent_backend agent_frontend agent_mcp_server >nul 2>&1

:: Build and start the containers
echo Building and starting containers with docker-compose...
docker-compose up -d --build

if %errorlevel% neq 0 (
    echo.
    echo ==================================================
    echo 容器启动失败 - Failed to start containers.
    echo.
    echo 如果您看到类似 Head https://registry-1.docker.io/... 的网络超时错误，
    echo 这通常是因为国内网络无法直接连接 Docker Hub。
    echo 请按照启动说明.md的指示，在您的 Docker Desktop 中配置网络代理 Proxy
    echo 或镜像加速器 Registry mirrors，然后再次运行本脚本。
    echo ==================================================
    pause
    exit /b 1
)

echo.
echo ==================================================
echo.
echo Containers started successfully!
echo.
echo Frontend web interface: http://localhost:18080
echo Backend API docs: http://localhost:18000/docs
echo.
echo Press any key to stop the containers...
pause >nul

echo.
echo Stopping containers...
docker-compose down
echo Containers stopped.
pause
