CREATE TYPE outage_kind AS ENUM (
  'start',
  'up',
  'down',
  'failover'
);

CREATE TABLE outages (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  ends_at timestamptz NOT NULL UNIQUE,
  kind outage_kind NOT NULL
);

INSERT INTO outages (ends_at, kind)
  VALUES (now(), 'start');
