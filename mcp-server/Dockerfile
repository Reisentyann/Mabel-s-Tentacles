FROM python:3.11-slim

WORKDIR /app

COPY pyproject.toml .
RUN pip install hatchling && pip install .

COPY . .

CMD ["python", "-m", "src.server"]
