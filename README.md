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
data, possible along the lines discussed in the [OSM Carto installation guide](https://github.com/gravitystorm/openstreetmap-carto/blob/master/INSTALL.md).

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
      RewriteRule /api/0.1/(.*) http://localhost:8081/$1 [P]
      RewriteRule /api/(.*) http://localhost:8081/$1 [P]
    </VirtualHost>

## Using

While GET requests are supported, POST requests are probably the better way 
to use the service. Here's a simple test query that will load fast food POIs
from your local osm2pgsql database:

    curl -g http://localhost:8081/interpreter --data-urlencode "data=
        SELECT name, way 
        FROM planet_osm_point
        WHERE amenity='fast_food' 
        AND way && st_setsrid(st_makebox2d(st_makepoint(8.34,48.97),st_makepoint(8.46,49.03)), 4326)"

