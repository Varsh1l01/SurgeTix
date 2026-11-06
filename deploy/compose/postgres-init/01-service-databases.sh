#!/bin/sh
# Creates one database and one login role per service that owns Postgres
# state (ADR-008: each service owns its schema; no service reads another's
# tables). Each role owns only its database, and PUBLIC loses CONNECT on
# every service database, so e.g. booking_svc cannot open checkout_db.
#
# Runs once, when the postgres-data volume is first initialised.
# Passwords come from the environment (.env); none are stored here.
set -eu

create_service_db() {
  role="$1" db="$2" password="$3"
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres \
       -v role="$role" -v db="$db" -v password="$password" <<'SQL'
CREATE ROLE :"role" LOGIN PASSWORD :'password';
CREATE DATABASE :"db" OWNER :"role";
REVOKE ALL ON DATABASE :"db" FROM PUBLIC;
GRANT CONNECT, TEMPORARY ON DATABASE :"db" TO :"role";
SQL
  echo "created database $db owned by $role"
}

create_service_db identity_svc identity_db "$IDENTITY_DB_PASSWORD"
create_service_db catalog_svc  catalog_db  "$CATALOG_DB_PASSWORD"
create_service_db booking_svc  booking_db  "$BOOKING_DB_PASSWORD"
create_service_db checkout_svc checkout_db "$CHECKOUT_DB_PASSWORD"
