# freegeoip

**A simplified fork with an embedded IP database**

A Docker image is automatically built once per month (on the 2nd of the month) with an embedded, updated version of the [dp-ip lite](https://db-ip.com/db/lite.php) database: [Repo on Docker Hub](https://hub.docker.com/r/dlebech/freegeoip).

Main changes in this fork:
- Use a new DB url from [dp-ip](https://db-ip.com/db/lite.php)
  - ***Auto-updating of the database has been removed!***
- Update to Go 1.18
- Use `go mod` for dependencies
- Remove custom http listener and use just the standard library http listener.
- Use [httprouter](https://github.com/julienschmidt/httprouter) for routing
- Remove XML and CSV outputs
- Remove everything that has to do with metrics (prometheus/newrelic)
- Remove auto-update of database
- Add "continent" to handler output
- Remove letsencrypt and TLS support
  - (so it's now mostly suitable for running behind a reverse proxy.)

***The below text is part of the original README***

---

This is the source code of the freegeoip software.

See http://en.wikipedia.org/wiki/Geolocation for details about geolocation.

## Running

This section is for people who desire to run the freegeoip web server on their own infrastructure. The easiest and most generic way of doing this is by using Docker. All examples below use Docker.

### Docker

#### Run the API in a container

```bash
docker run --restart=always -p 8080:8080 -d dlebech/freegeoip
```

#### Test

```bash
curl localhost:8080/json/1.2.3.4
# => {"ip":"1.2.3.4","country_code":"US","country_name":"United States", # ...
```

### Production configuration

For production workloads you may want to use different configuration for the freegeoip web server, for example:

* Configuring the read and write timeouts to avoid stale clients consuming server resources
* Configuring the freegeoip web server to read the client IP (for logs, etc) from the X-Forwarded-For header when running behind a reverse proxy

### Server Options

To see all the available options, use the `-help` option:

```bash
docker run --rm -it dlebech/freegeoip -help
```

You can configure the freegeiop web server via command line flags or environment variables. The names of environment variables are the same for command line flags, all upperscase, separated by underscores. If you want to use environment variables instead:

```bash
$ cat prod.env
PORT=8888

$ docker run --env-file=prod.env -p 8888:8888 -p -d dlebech/freegeoip
```

If the freegeoip web server is running behind a reverse proxy or load balancer, you have to run it passing the `-use-x-forwarded-for` parameter and provide the `X-Forwarded-For` HTTP header in all requests. This is for the freegeoip web server be able to log the client IP, and to perform geolocation lookups when an IP is not provided to the API, e.g. `/json/` (uses client IP) vs `/json/1.2.3.4`.

## Database

The current implementation uses the free [dp-ip](https://db-ip.com/db/lite.php) database that has a similar format to the one from MaxMind.

**This database is built into the Docker container and does not auto-update**

All responses from the freegeiop API contain the date that the database was downloaded in the X-Database-Date HTTP header.

## API

The freegeoip API is served by endpoints that encode the response in different formats.

Example:

```bash
curl example.com/json/
```

Returns the geolocation information of your own IP address, the source IP address of the connection.

You can pass a different IP or hostname. For example, to lookup the geolocation of `github.com` the server resolves the name first, then uses the first IP address available, which might be IPv4 or IPv6:

```bash
curl example.com/json/github.com
```