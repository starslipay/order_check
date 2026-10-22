# Minute-level: check every 60 seconds, time window last 5 minutes, repair inconsistency by calling trade_itg
$APP_NAME = "order_check-minute"
$IMAGE_NAME = "order_check"
$TAG = "minute"
$CONFIG_FILE = "etc/config_minute.yaml"

docker rm -f $APP_NAME 2>$null
docker rmi -f "${IMAGE_NAME}:${TAG}" 2>$null

Write-Host "Building Docker image for minute-level..."
docker build -t "${IMAGE_NAME}:${TAG}" --build-arg CONFIG_FILE=$CONFIG_FILE .

if ($LASTEXITCODE -eq 0) {
    Write-Host "Build successful"
} else {
    Write-Host "Build failed"
    exit 1
}

Write-Host "Starting container..."
docker run -d --name $APP_NAME --network dev_pay_net -e TZ=Asia/Shanghai "${IMAGE_NAME}:${TAG}"

if ($LASTEXITCODE -eq 0) {
    Write-Host "Container started"
} else {
    Write-Host "Container start failed"
    exit 1
}

docker ps --filter "name=$APP_NAME" --format "{{.Names}}`t{{.Status}}"
