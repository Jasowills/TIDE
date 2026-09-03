FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/tide-api ./cmd/tide-api && \
    CGO_ENABLED=0 go build -o /out/tide-engine ./cmd/tide-engine && \
    CGO_ENABLED=0 go build -o /out/tide ./cmd/tide && \
    CGO_ENABLED=0 go build -o /out/tide-sim ./cmd/tide-sim

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/tide-api /out/tide-engine /out/tide /out/tide-sim /usr/local/bin/
COPY simulator/scenarios /etc/tide/scenarios
COPY rules /etc/tide/rules
ENV TIDE_SCENARIOS=/etc/tide/scenarios TIDE_RULES=/etc/tide/rules
EXPOSE 8080 8081
