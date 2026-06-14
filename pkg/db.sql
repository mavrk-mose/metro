-- Seoul Metro database schema

CREATE EXTENSION IF NOT EXISTS postgis;

-- Lines table
CREATE TABLE IF NOT EXISTS lines (
  id          VARCHAR(8) PRIMARY KEY,
  name_ko     TEXT NOT NULL,
  name_en     TEXT NOT NULL,
  color_hex   VARCHAR(7),
  is_circular BOOLEAN DEFAULT FALSE
);

-- Stations table
CREATE TABLE IF NOT EXISTS stations (
  id          VARCHAR(8) PRIMARY KEY,
  name_ko     TEXT NOT NULL,
  name_en     TEXT NOT NULL,
  location    GEOMETRY(POINT, 4326) NOT NULL,
  is_transfer BOOLEAN DEFAULT FALSE,
  zone_id     INT NOT NULL DEFAULT 1,
  exits       INT DEFAULT 1,
  created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Create spatial index on location
CREATE INDEX IF NOT EXISTS idx_stations_location ON stations USING GIST(location);
CREATE INDEX IF NOT EXISTS idx_stations_name_ko ON stations USING GIN(to_tsvector('korean', name_ko));
CREATE INDEX IF NOT EXISTS idx_stations_name_en ON stations USING GIN(to_tsvector('english', name_en));

-- Edges (connections between stations)
CREATE TABLE IF NOT EXISTS edges (
  id            SERIAL PRIMARY KEY,
  from_id       VARCHAR(8) NOT NULL REFERENCES stations(id),
  to_id         VARCHAR(8) NOT NULL REFERENCES stations(id),
  line_id       VARCHAR(8) NOT NULL REFERENCES lines(id),
  travel_secs   INT NOT NULL,
  is_transfer   BOOLEAN DEFAULT FALSE,
  transfer_secs INT DEFAULT 0,
  UNIQUE(from_id, to_id, line_id)
);

CREATE INDEX IF NOT EXISTS idx_edges_from ON edges(from_id);
CREATE INDEX IF NOT EXISTS idx_edges_to ON edges(to_id);
CREATE INDEX IF NOT EXISTS idx_edges_line ON edges(line_id);

-- Schedules (timetables)
CREATE TABLE IF NOT EXISTS schedules (
  id           SERIAL PRIMARY KEY,
  line_id      VARCHAR(8) NOT NULL REFERENCES lines(id),
  station_id   VARCHAR(8) NOT NULL REFERENCES stations(id),
  direction    VARCHAR(64),
  arrival_time TIME NOT NULL,
  day_type     VARCHAR(10) NOT NULL, -- 'weekday', 'saturday', 'sunday'
  created_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_schedules_stn_line_day ON schedules(station_id, line_id, day_type);
CREATE INDEX IF NOT EXISTS idx_schedules_arrival_time ON schedules(arrival_time);

-- Fare zones
CREATE TABLE IF NOT EXISTS fare_zones (
  zone_id         INT PRIMARY KEY,
  base_fare       INT NOT NULL,
  per_km_rate     NUMERIC(6, 2) NOT NULL,
  transfer_discount NUMERIC(4, 2) DEFAULT 0.10,
  created_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Users (for saved routes, preferences)
CREATE TABLE IF NOT EXISTS users (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  home_station    VARCHAR(8) REFERENCES stations(id),
  saved_routes    JSONB DEFAULT '[]',
  tmoney_card     VARCHAR(32),
  pref_mode       VARCHAR(16) DEFAULT 'fastest', -- fastest, least_transfer, least_walk
  created_at      TIMESTAMPTZ DEFAULT NOW(),
  updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Sample data: Lines
INSERT INTO lines (id, name_ko, name_en, color_hex, is_circular) VALUES
  ('1', '서울 지하철 1호선', 'Seoul Metro Line 1', '#0052CC', FALSE),
  ('2', '서울 지하철 2호선', 'Seoul Metro Line 2', '#009D3E', TRUE),
  ('3', '서울 지하철 3호선', 'Seoul Metro Line 3', '#EF7C1C', FALSE),
  ('4', '서울 지하철 4호선', 'Seoul Metro Line 4', '#0084D6', FALSE),
  ('5', '서울 지하철 5호선', 'Seoul Metro Line 5', '#996644', FALSE),
  ('6', '서울 지하철 6호선', 'Seoul Metro Line 6', '#CD7C2F', FALSE),
  ('7', '서울 지하철 7호선', 'Seoul Metro Line 7', '#747F00', FALSE),
  ('8', '서울 지하철 8호선', 'Seoul Metro Line 8', '#E6186C', FALSE),
  ('9', '서울 지하철 9호선', 'Seoul Metro Line 9', '#AA9D3A', FALSE),
  ('BD', '신분당선', 'Bundang-Sinbundang Line', '#ED5E0E', FALSE),
  ('SBD', '신분당선', 'Sinbundang Line', '#D41159', FALSE),
  ('AREX', '공항철도', 'Airport Railroad', '#0066CC', FALSE)
ON CONFLICT DO NOTHING;

-- Sample stations (Seoul Station, Hongik Univ., Gangnam, etc.)
INSERT INTO stations (id, name_ko, name_en, location, is_transfer, zone_id) VALUES
  ('0150', '서울역', 'Seoul Station', ST_GeomFromText('POINT(126.9705 37.5548)', 4326), TRUE, 1),
  ('0222', '홍대입구', 'Hongik Univ.', ST_GeomFromText('POINT(126.9242 37.5545)', 4326), FALSE, 1),
  ('0519', '강남역', 'Gangnam Station', ST_GeomFromText('POINT(127.0276 37.4979)', 4326), TRUE, 1),
  ('0212', '신도림', 'Sindorim', ST_GeomFromText('POINT(126.8948 37.5092)', 4326), TRUE, 1),
  ('0139', '종로3가', 'Jongno 3-ga', ST_GeomFromText('POINT(126.9912 37.5704)', 4326), TRUE, 1),
  ('0126', '반포', 'Banpo', ST_GeomFromText('POINT(126.9978 37.5302)', 4326), FALSE, 1),
  ('0401', '동대문', 'Dongdaemun', ST_GeomFromText('POINT(127.0091 37.5662)', 4326), TRUE, 1),
  ('0901', '명동', 'Myeongdong', ST_GeomFromText('POINT(126.9846 37.5639)', 4326), FALSE, 1),
  ('0701', '강변', 'Gangbyeon', ST_GeomFromText('POINT(127.0799 37.5385)', 4326), FALSE, 1),
  ('0304', '명일', 'Myeongil', ST_GeomFromText('POINT(127.1068 37.5471)', 4326), FALSE, 1)
ON CONFLICT DO NOTHING;

-- Sample edges (connections - simplified example)
INSERT INTO edges (from_id, to_id, line_id, travel_secs, is_transfer, transfer_secs) VALUES
  -- Line 1: Seoul Station to Jongno 3-ga
  ('0150', '0139', '1', 180, FALSE, 0),
  ('0139', '0150', '1', 180, FALSE, 0),
  ('0139', '0401', '1', 300, FALSE, 0),
  ('0401', '0139', '1', 300, FALSE, 0),
  
  -- Line 2: Transfer points and connections
  ('0150', '0212', '2', 420, FALSE, 0),
  ('0212', '0519', '2', 900, FALSE, 0),
  ('0519', '0126', '2', 600, FALSE, 0),
  
  -- Transfer at Seoul Station (Line 1 <-> Line 4)
  ('0150', '0150', '4', 300, TRUE, 240),
  ('0150', '0150', '1', 300, TRUE, 240),
  
  -- More connections...
  ('0222', '0901', '2', 1200, FALSE, 0),
  ('0901', '0222', '2', 1200, FALSE, 0)
ON CONFLICT DO NOTHING;

-- Sample schedules (weekday, peak hour)
INSERT INTO schedules (line_id, station_id, direction, arrival_time, day_type) VALUES
  ('1', '0150', '인천', '08:00'::time, 'weekday'),
  ('1', '0150', '인천', '08:05'::time, 'weekday'),
  ('1', '0150', '인천', '08:10'::time, 'weekday'),
  ('1', '0139', '인천', '08:03'::time, 'weekday'),
  ('1', '0139', '인천', '08:08'::time, 'weekday'),
  ('2', '0212', '신도림', '08:02'::time, 'weekday'),
  ('2', '0212', '신도림', '08:07'::time, 'weekday')
ON CONFLICT DO NOTHING;

-- Fare zones (Seoul uses distance-based fares)
INSERT INTO fare_zones (zone_id, base_fare, per_km_rate, transfer_discount) VALUES
  (1, 1550, 100, 0.10)
ON CONFLICT DO NOTHING;
