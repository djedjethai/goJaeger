-- create-schema.cql
CREATE KEYSPACE IF NOT EXISTS jaeger_v1_dc1 WITH REPLICATION = {
  'class' : 'SimpleStrategy',
  'replication_factor' : 1
};

USE jaeger_v1_dc1;

CREATE TABLE IF NOT EXISTS traces (
  trace_id UUID PRIMARY KEY,
  span_id UUID,
  parent_id UUID,
  operation_name text,
  flags int,
  start_time timestamp,
  duration int,
  tags map<text, text>,
  logs list<frozen<map<text, text>>>,
  references list<frozen<map<text, text>>>,
  process_id UUID,
  service_name text
);

