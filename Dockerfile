ARG SHELFY_RUNTIME_BASE_IMAGE=shelfy-runtime-base:vosk-small-ru-0.22

FROM golang:1.26 AS build

WORKDIR /app
ARG TARGETOS=linux
ARG TARGETARCH
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/telegram-api ./cmd/telegram-api
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/pipeline-worker ./cmd/pipeline-worker
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/scheduler-worker ./cmd/scheduler-worker
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/migrate ./cmd/migrate

FROM ${SHELFY_RUNTIME_BASE_IMAGE}

WORKDIR /app
COPY --from=build /out/telegram-api /usr/local/bin/telegram-api
COPY --from=build /out/pipeline-worker /usr/local/bin/pipeline-worker
COPY --from=build /out/scheduler-worker /usr/local/bin/scheduler-worker
COPY --from=build /out/migrate /usr/local/bin/migrate
COPY scripts/vosk_transcribe.py /usr/local/bin/vosk-transcribe
COPY migrations ./migrations
COPY copy ./copy
COPY docs ./docs

ENV SHELFY_TMP_DIR=/tmp/shelfy
RUN chmod +x /usr/local/bin/vosk-transcribe && mkdir -p /tmp/shelfy /models

CMD ["telegram-api"]
