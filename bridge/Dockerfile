FROM golang:1.23-alpine AS build

WORKDIR /src

COPY go.mod ./
COPY main.go ./
COPY internal ./internal

RUN go mod download && go build -o /out/bridge ./main.go

FROM alpine:3.20

WORKDIR /app
COPY --from=build /out/bridge /app/bridge

CMD ["/app/bridge"]
