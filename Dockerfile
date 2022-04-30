FROM golang:1.18

ARG DATABASE_URL

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
COPY apiserver ./apiserver
COPY cmd ./cmd

RUN go build -o /freegeoip ./cmd/freegeoip

RUN curl -fSLo db.gz $DATABASE_URL

ENTRYPOINT ["/freegeoip"]
