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

    go build -o postpass-server postpass/main.go

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
    grant select on all tables in schema public to readonly;

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
      ServerName my.postpass.server

      DocumentRoot /var/www/html

      ProxyTimeout 3600
      RewriteEngine on
      RewriteRule /api/0.2/(.*) http://localhost:8081/$1 [P]
      RewriteRule /api/(.*) http://localhost:8081/$1 [P]
    </VirtualHost>

## Using

### Curl

While GET requests are supported, POST requests are probably the better way 
to use the service. Here's a simple test query that will load fast food POIs
from your local osm2pgsql database:

    curl -g https://postpass.geofabrik.de/api/0.2/interpreter --data-urlencode "data=
        SELECT name, way 
        FROM planet_osm_point
        WHERE amenity='fast_food' 
        AND way && st_setsrid(st_makebox2d(st_makepoint(8.34,48.97),st_makepoint(8.46,49.03)), 4326)"

### LLM

This prompt helps to generate good results with LLMs like ChatGPT.


> Please generate a `curl` command containing a SQL query that will be sent to the [Postpass API](https://github.com/woodpeck/postpass), which exposes a PostGIS-enabled PostgreSQL database with OpenStreetMap data.
>
> The API endpoint is `https://postpass.geofabrik.de/api/0.2/interpreter`
>
> The underlying database schema is described at: https://github.com/woodpeck/postpass-ops/blob/main/SCHEMA.md
>
> The database uses the `osm2pgsql flex` schema, storing tags as `jsonb`. You have access to the following main geometry tables:
>
> * `postpass_point` (geometry: Point)
> * `postpass_line` (geometry: MultiLineString)
> * `postpass_polygon` (geometry: MultiPolygon)
>
> Additionally, combined geometry views are available:
>
> * `postpass_pointpolygon`
> * `postpass_pointline`
> * `postpass_linepolygon`
> * `postpass_pointlinepolygon`
>
> Tags are stored in a `jsonb` column named `tags`. Use `tags->>'key'` to retrieve values or `tags ? 'key'` to check for tag presence.
>
> By default, the API returns results as GeoJSON (geometry included). If geometry is **not** required, include the following in the request: `--data-urlencode "options[geojson]=false"`. Use `geojson=false` when: No geometry column (`geom`) is selected; You are returning aggregated results (e.g., `COUNT`, `GROUP BY`).
>
> ---
>
> Examples:
>
> 1. Return geometries (default GeoJSON):
>
> > curl -g [https://postpass.geofabrik.de/api/0.2/interpreter](https://postpass.geofabrik.de/api/0.2/interpreter) --data-urlencode "data=
> > SELECT name, geom
> > FROM postpass\_point
> > WHERE tags->>'amenity' = 'fast\_food'
> > AND geom && ST\_SetSRID(ST\_MakeBox2D(ST\_MakePoint(8.34, 48.97), ST\_MakePoint(8.46, 49.03)), 4326)"
>
> 2. Return aggregated result (no geometry, use `geojson=false`):
>
> > curl -g [https://postpass.geofabrik.de/api/0.2/interpreter](https://postpass.geofabrik.de/api/0.2/interpreter)
> > \--data-urlencode "options\[geojson]=false"
> > \--data-urlencode "data=
> > SELECT
> > admin.tags->>'name' AS country,
> > COUNT(point.\*) AS ref\_count
> > FROM postpass\_point AS point
> > JOIN postpass\_polygon AS admin
> > ON ST\_Contains(admin.geom, point.geom)
> > WHERE
> > point.tags->>'natural' = 'tree'
> > AND point.tags ? 'ref'
> > AND admin.tags->>'boundary' = 'administrative'
> > AND admin.tags->>'admin\_level' = '2'
> > GROUP BY admin.tags->>'name'"
>
> ---
>
> Always:
>
> * Return a full `curl` command
> * Never include a `;` at the end of the SQL query.
> * Use the correct table or view based on the geometry type requested
> * Add `geojson=false` whenever geometry is **not** requested in the result set
>
> ---
> MY QUESTION:
> …
