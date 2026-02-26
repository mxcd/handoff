# Stage 2: Build Go binary with embedded frontend
FROM golang:1.25 AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=development
ARG COMMIT=unknown
RUN CGO_ENABLED=0 go build -ldflags "-X github.com/mxcd/handoff/internal/util.Version=${VERSION} -X github.com/mxcd/handoff/internal/util.Commit=${COMMIT}" -o /server ./cmd/server

# Stage 3: Minimal runtime
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=go-builder /server /server
ENTRYPOINT ["/server"]
