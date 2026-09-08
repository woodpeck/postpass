# Postpass

A simple wrapper around PostGIS that allows random people on the
internet to run PostGIS queries without ruining everything

This is inspired by the great success in OpenStreetMap circles of the 
[Overpass API](https://github.com/drolbr/Overpass-API) together with 
[Overpass Turbo](https://github.com/tyrasd/overpass-turbo). 
Postpass is intended to do approximately the same things that Overpass API
does, just based on a PostGIS database.

While all documentation assumes that you will want to use this with 
an OpenStreetMap database, Postpass itself is totally agnostic about 
the data you have in your database and you could theoretically use it
with anything else as well.

See [woodpeck/postpass-ops](https://github.com/woodpeck/postpass-ops) for 
docs on the instance of this software at `postpass.geofabrik.de`.

## Building

A simple

    go build -o postpass-server cmd/postpass/main.go

should do the trick.

You can also type

    make

You will need go v1.24 for this. 

## Setup and Installation

You want a local PostGIS database with some sort of OpenStreetMap 
data import. We assume that you will use 
[osm2pgsql](https://github.com/osm2pgsql-dev/osm2pgsql) to import your
data, possibly along the lines discussed in the [OSM Carto installation guide](https://github.com/gravitystorm/openstreetmap-carto/blob/master/INSTALL.md). (The instance on postpass.geofabrik.de uses a slightly different data schema, see [woodpeck/postpass-ops](https://github.com/woodpeck/postpass-ops) for details.)

It is very much recommended to create a read-only user in your database,
else you'll have random people on the Internet doing the "little bobby tables"
joke on you:

    create user readonly with password 'readonly';
    grant usage on schema public to readonly;
    grant select on all tables in schema public to readonly ;
    alter default privileges in schema public grant select on tables to readonly;

You could now start the Postpass software manually (it will listen on
its own port, by default 8081); for a halfway reliable production environment
you will probably want to create a unix user `postpass` and a systemd service like so:

    [Unit]
    Description=postpass server
    StartLimitIntervalSec=0
    StartLimitInterval=0

    [Service]
    User=postpass
    ExecStart=/srv/postpass/postpass-server # or whereever your binary is
    StandardOutput=journal
    StandardError=journal
    Restart=on-failure
    RestartSec=10

    [Install]
    WantedBy=multi-user.target

and you will probably want to configure a standard web server to sit
in front of Postpass, for example Apache:

    <VirtualHost *:80>
      ServerName postpass.example.com

      DocumentRoot /var/www/html

      ProxyTimeout 3600
      RewriteEngine on
      RewriteRule /api/(.*) http://localhost:8081/$1 [P]
      RewriteRule /api/(.*) http://localhost:8081/$1 [P]
    </VirtualHost>

## Configuration

postpass can be configured with a configuration file.
The location of the config file can be specified using the `-c` flag.
If no file is provided or values are not set, default values are assumed.
To see the default values run `postpass-server --print-config`.
To create a config file with default values run `postpass-server --print-config > postpass.yaml`

The database config can also be overridden using cli arguments (check `postpass-server -h`).
Those values are applied after the config has been loaded, so the values will override the config.

```yaml
database:
  host: localhost
  port: 5432
  user: readonly
  password: readonly
  database_name: gis
listen_port: 8081
quick_medium_threshold: 150
medium_slow_threshold: 150000
```

## Dev setup (for simple testing)

> [!WARNING]
> This is just for local testing and not secure in any way.
> Please set permissions for the user used by postpass, otherwise users can edit your data.

1. Download [postpass.lua](https://github.com/woodpeck/postpass-ops/blob/main/postpass.lua)
1. Download any [`.osm.pbf`](https://download.geofabrik.de/)
1. Build postpass (`make`)
1. Run the following commands from different terminals

```shell
podman run --rm --network host -e POSTGRES_USER=postpass -e POSTGRES_PASSWORD=password docker.io/postgis/postgis:latest
# set data folder to the actual location
podman run --rm --network host -v ~/Downloads:/data:ro -e PGHOST=localhost -e PGUSER=postpass -e PGPASSWORD=password docker.io/iboates/osm2pgsql:latest -O flex -S /data/postpass.lua /data/data.osm.pbf
./postpass-server --db-username postpass --db-password password --db-name postpass
```

## Using

### `/interpreter`

The main query end point is `/interpreter`.

GET and POST requests are supported. Here's a simple test query that will load
fast food POIs from your local osm2pgsql database. (schema is based on
[postpass.geofabrik.de schema](https://github.com/woodpeck/postpass-ops)).

    curl -G http://localhost:8081/interpreter --data-urlencode "q=
        SELECT tags->>'name' as name, geom 
        FROM postpass_point
        WHERE tags->>'amenity'='fast_food' 
        AND geom && st_setsrid(st_makebox2d(st_makepoint(8.34,48.97),st_makepoint(8.46,49.03)), 4326)"

Large queries can be saved in a separate file. Use curl's `@filename` option to
read that file. Here the conents of the file `query.sql` will be read.

    curl -G http://localhost:8081/interpreter --data-urlencode "data@query.sql"

### `/explain`

A [PostgreSQL `EXPLAIN` output](https://www.postgresql.org/docs/current/sql-explain.html) is returned, in [JSON output format](https://www.postgresql.org/docs/current/sql-explain.html#:~:text=JSON%20output%20formatting%3A).

e.g.:

    curl -G http://localhost:8081/explain --data-urlencode "q=SELECT tags->>'name' as name, geom FROM postpass_point"

### `/metrics`

Prometheus metrics. Default turned on. Can be disabled with `metrics.enabled = false` in the config file.


#### `cache_for`

For `GET` requests, if the parameter `cache_for` is a number, then the response
will include a `Cache-Control: max-age=X` HTTP header.

e.g.: The following response will have `Cache-Control: max-age=10000` response header.

    curl -G http://localhost:8081/interpreter --data-urlencode "cache_for=10000" --data-urlencode "data=SELECT tags->>'name' as name, geom FROM postpass_point limit 1"

> [!IMPORTANT]
> postpass does not do any caching of responses. This option merely allows the
> user to specify a header that any intermediate proxies might use.

If `cache_for` is unset, `options[cache_for]` will be checked. These options
have no effect on `POST` requests. Very high `cache_for` settings will be
clamped to a sensible max value (currently: 2 days).

### `/healthy`

Returns HTTP 200 and the text `healthy\n`. Useful for automated health checks.

### LLM

This prompt helps to generate good results with LLMs like ChatGPT.


```
Please generate a `curl` command containing a SQL query that will be sent to the [Postpass API](https://github.com/woodpeck/postpass), which exposes a PostGIS-enabled PostgreSQL database with OpenStreetMap data.

The API endpoint is `https://postpass.geofabrik.de/api/interpreter`

The underlying database schema is described at: https://github.com/woodpeck/postpass-ops/blob/main/SCHEMA.md

The database uses the `osm2pgsql flex` schema, storing tags as `jsonb`. You have access to the following main geometry tables:

* `postpass_point` (geometry: Point)
* `postpass_line` (geometry: MultiLineString)
* `postpass_polygon` (geometry: MultiPolygon)

Additionally, combined geometry views are available:

* `postpass_pointpolygon`
* `postpass_pointline`
* `postpass_linepolygon`
* `postpass_pointlinepolygon`

Tags are stored in a `jsonb` column named `tags`. Use `tags->>'key'` to retrieve values or `tags ? 'key'` to check for tag presence.

By default, the API returns results as GeoJSON (geometry included). If geometry is **not** required, include the following in the request: `--data-urlencode "options[geojson]=false"`. Use `geojson=false` when: No geometry column (`geom`) is selected; You are returning aggregated results (e.g., `COUNT`, `GROUP BY`).

---

Examples:

1. Return geometries (default GeoJSON):

> curl -G https://postpass.geofabrik.de/api/interpreter --data-urlencode "q=
> SELECT name, geom
> FROM postpass\_point
> WHERE tags->>'amenity' = 'fast\_food'
> AND geom && ST\_SetSRID(ST\_MakeBox2D(ST\_MakePoint(8.34, 48.97), ST\_MakePoint(8.46, 49.03)), 4326)"

2. Return aggregated result (no geometry, use `geojson=false`):

> curl -G https://postpass.geofabrik.de/api/interpreter
> \--data-urlencode "options\[geojson]=false"
> \--data-urlencode "q=
> SELECT
> admin.tags->>'name' AS country,
> COUNT(point.\*) AS ref\_count
> FROM postpass\_point AS point
> JOIN postpass\_polygon AS admin
> ON ST\_Contains(admin.geom, point.geom)
> WHERE
> point.tags->>'natural' = 'tree'
> AND point.tags ? 'ref'
> AND admin.tags->>'boundary' = 'administrative'
> AND admin.tags->>'admin\_level' = '2'
> GROUP BY admin.tags->>'name'"

---

Always:

* Return a full `curl` command
* Never include a `;` at the end of the SQL query.
* Use the correct table or view based on the geometry type requested
* Add `geojson=false` whenever geometry is **not** requested in the result set

---
MY QUESTION:
…
```
