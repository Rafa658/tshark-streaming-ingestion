FROM golang:1.26.5-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags='-s -w' -o /worker ./cmd/worker

FROM scratch
COPY --from=build /worker /worker
ENTRYPOINT ["/worker"]
