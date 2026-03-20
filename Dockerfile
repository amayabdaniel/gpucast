FROM golang:1.22 AS builder
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o gpucast .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /workspace/gpucast /usr/local/bin/gpucast
USER 65532:65532
EXPOSE 9400
ENTRYPOINT ["gpucast"]
