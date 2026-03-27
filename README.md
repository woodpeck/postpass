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
      RewriteRule /api/0.2/(.*) http://localhost:8081/$1 [P]
      RewriteRule /api/(.*) http://localhost:8081/$1 [P]
    </VirtualHost>

## Using

### `/interpreter`

The main query end point is `/interpreter`.

GET and POST requests are supported. Here's a simple test query that will load
fast food POIs from your local osm2pgsql database. (schema is based on
[postpass.geofabrik.de schema](https://github.com/woodpeck/postpass-ops)).

    curl -G http://localhost:8081/interpreter --data-urlencode "data=
        SELECT tags->>'name' as name, geom 
        FROM postpass_point
        WHERE tags->>'amenity'='fast_food' 
        AND geom && st_setsrid(st_makebox2d(st_makepoint(8.34,48.97),st_makepoint(8.46,49.03)), 4326)"

Large queries can be saved in a separate file. Use curl's `@filename` option to
read that file. Here the conents of the file `query.sql` will be read.

    curl -G http://localhost:8081/interpreter --data-urlencode "data@query.sql"

#### Output format

By default, a single GeoJSON `FeatureCollection` is returned. This can be changed with the `output_foramt` parameter.

e.g.

    curl -G http://localhost:8081/interpreter --data-urlencode "output_format=md_table" --data-urlencode "data=SELECT count(*), tags->>'name' IS NULL as is_null from postpass_point group by is_null"
    | count   | is_null |
    |---------|---------|
    | 141183  | false   |
    | 2404499 | true    |

Acceptable values:

* **`geojson`**: (default if not defined) single GeoJSON `FeatureCollection`.
  If each row doesn't have a geometry column, a `HTTP 400` will be returned.
* **`html_table`**: A table in HTML
  ```html
  <table>
  <thead><th>count</th><th>is_null</th></thead>
  <tr><td>141183</td><td>false</td></tr>
  <tr><td>2404499</td><td>true</td></tr>
  </table>
  ```
* **`md_table`**: A table in Markdown
  ```markdown
  | count   | is_null |
  |---------|---------|
  | 141183  | false   |
  | 2404499 | true    |
  ```
* **`json`**: JSON Array of Objects.
   In the above example: `[{"count":141183,"is_null":false}, {"count":2404499,"is_null":true}]`.
   NB: This uses [PostgreSQL's `to_json` function](https://www.postgresql.org/docs/current/functions-json.html#:~:text=to%5Fjson%20%28%20anyelement%20%29%20%E2%86%92%20json), which converts geometry objects to GeoJSON
   ```
   $ curl  -G http://localhost:8081/interpreter --data-urlencode "output_format=json" --data-urlencode "data=SELECT tags->>'name' as name, geom from postpass_point limit 1" ; echo
   [{"name":null,"geom":{"type":"Point","crs":{"type":"name","properties":{"name":"EPSG:4326"}},"coordinates":[-6.6822123,55.1341014]}}]
   ```
* **`csv`**: Comma Separated Values
  ```csv
  count,is_null
  141183,false
  2404499,true
  ```
* **`csv_headerless`**: Comma Separated Values without a header row
  ```csv
  141183,false
  2404499,true
  ```
* **`tsv`**: Tab Seperated Values
  ```tsv
  count	is_null
  141183	false
  2404499	true
  ```
* **`tsv_headerless`**: TSV without header row
* **`sql_values`**: [SQL `VALUES`](https://www.postgresql.org/docs/current/sql-values.html) statement
  ```sql
  VALUES (141183, false), (2404499, true)
  ```
* **`with_sql_values`**: [SQL `VALUES`](https://www.postgresql.org/docs/current/sql-values.html) with the header columns, suitable to copy/paste into a `WITH` statement for later usage
  ```sql
  ("count", "is_null") AS (VALUES (141183, false), (2404499, true))
  ```
  This can be used later like:
  ```sql
  WITH original_values ("count", "is_null") AS (VALUES (141183, false), (2404499, true)),
  select * from data join original_values ...
  ```



### `/explain`

A [PostgreSQL `EXPLAIN` output](https://www.postgresql.org/docs/current/sql-explain.html) is returned, in [JSON output format](https://www.postgresql.org/docs/current/sql-explain.html#:~:text=JSON%20output%20formatting%3A).

e.g.:

    curl -G http://localhost:8081/explain --data-urlencode "data=SELECT tags->>'name' as name, geom FROM postpass_point"

### LLM

This prompt helps to generate good results with LLMs like ChatGPT.


```
Please generate a `curl` command containing a SQL query that will be sent to the [Postpass API](https://github.com/woodpeck/postpass), which exposes a PostGIS-enabled PostgreSQL database with OpenStreetMap data.

The API endpoint is `https://postpass.geofabrik.de/api/0.2/interpreter`

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

> curl -G [https://postpass.geofabrik.de/api/0.2/interpreter](https://postpass.geofabrik.de/api/0.2/interpreter) --data-urlencode "data=
> SELECT name, geom
> FROM postpass\_point
> WHERE tags->>'amenity' = 'fast\_food'
> AND geom && ST\_SetSRID(ST\_MakeBox2D(ST\_MakePoint(8.34, 48.97), ST\_MakePoint(8.46, 49.03)), 4326)"

2. Return aggregated result (no geometry, use `geojson=false`):

> curl -G [https://postpass.geofabrik.de/api/0.2/interpreter](https://postpass.geofabrik.de/api/0.2/interpreter)
> \--data-urlencode "options\[geojson]=false"
> \--data-urlencode "data=
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
