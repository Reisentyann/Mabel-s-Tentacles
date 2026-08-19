FROM python:3.11-slim

WORKDIR /app

COPY pyproject.toml .
RUN pip config set global.index-url https://mirrors.aliyun.com/pypi/simple/ \
    && pip install hatchling \
    && pip install .

COPY . .

CMD ["python", "-m", "src.server"]
