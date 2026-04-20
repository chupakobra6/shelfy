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

FROM debian:bookworm AS whisper-build

ARG WHISPER_CPP_REF=v1.8.4

RUN apt-get update \
	&& apt-get install -y --no-install-recommends build-essential ca-certificates cmake git \
	&& rm -rf /var/lib/apt/lists/*

WORKDIR /src
RUN git clone --branch ${WHISPER_CPP_REF} --depth 1 https://github.com/ggml-org/whisper.cpp.git .
RUN cmake -B build \
	-DWHISPER_SDL2=OFF \
	-DWHISPER_BUILD_TESTS=OFF \
	-DGGML_CPU_REPACK=OFF \
	-DGGML_NATIVE=OFF
RUN cmake --build build -j --config Release --target whisper-cli

FROM debian:bookworm-slim

RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates ffmpeg tesseract-ocr tesseract-ocr-rus \
	&& rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=build /out/telegram-api /usr/local/bin/telegram-api
COPY --from=build /out/pipeline-worker /usr/local/bin/pipeline-worker
COPY --from=build /out/scheduler-worker /usr/local/bin/scheduler-worker
COPY --from=build /out/migrate /usr/local/bin/migrate
COPY --from=whisper-build /src/build/bin/whisper-cli /usr/local/bin/whisper-cli
COPY migrations ./migrations
COPY copy ./copy
COPY docs ./docs

ENV SHELFY_TMP_DIR=/tmp/shelfy
RUN mkdir -p /tmp/shelfy /models

CMD ["telegram-api"]
