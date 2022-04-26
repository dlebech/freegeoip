FROM golang:1.18-alpine

ARG DATABASE_URL

WORKDIR /app

COPY go.mod go.sum
RUN go mod download

COPY *.go ./
COPY apiserver ./apiserver
COPY cmd ./cmd

RUN go build -o /freegeoip ./cmd/freegeoip

RUN curl -fSLo db.gz $DATABASE_URL

ENTRYPOINT ["/freegeoip"]

EXPOSE 8080

# CMD instructions:
# Add  "-use-x-forwarded-for"      if your server is behind a reverse proxy
# Add  "-public", "/app/cmd/freegeoip/public"       to enable the web front-end
#
# Example:
# CMD ["-use-x-forwarded-for", "-public", "/app/cmd/freegeoip/public"]
