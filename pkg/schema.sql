CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE stations (
  id          VARCHAR(8) PRIMARY KEY,        -- e.g. "0150"
    name_ko     TEXT NOT NULL,                 -- 서울역
      name_en     TEXT NOT NULL,                 -- Seoul Station
        location    GEOMETRY(POINT, 4326),         -- WGS84
          is_transfer BOOLEAN DEFAULT FALSE,
            zone_id     INT NOT NULL,
              exits       INT DEFAULT 1
              );
              CREATE INDEX ON stations USING GIST(location);  -- spatial index

              CREATE TABLE lines (
                id        VARCHAR(8) PRIMARY KEY,  -- "1","2","BD","AREX"
                  name_ko   TEXT,
                    color_hex VARCHAR(7),
                      is_circular BOOLEAN DEFAULT FALSE  -- Line 2 is circular
                      );

                      CREATE TABLE edges (
                        from_id       VARCHAR(8) REFERENCES stations(id),
                          to_id         VARCHAR(8) REFERENCES stations(id),
                            line_id       VARCHAR(8) REFERENCES lines(id),
                              travel_secs   INT NOT NULL,
                                is_transfer   BOOLEAN DEFAULT FALSE,
                                  transfer_secs INT DEFAULT 0,    -- platform-walk penalty
                                    PRIMARY KEY (from_id, to_id, line_id)
                                    );

                                    CREATE TABLE schedules (
                                      id           SERIAL PRIMARY KEY,
                                        line_id      VARCHAR(8) REFERENCES lines(id),
                                          station_id   VARCHAR(8) REFERENCES stations(id),
                                            direction    VARCHAR(64),        -- e.g. "수원" (Suwon-bound)
                                              arrival_time TIME NOT NULL,
                                                day_type     VARCHAR(8) NOT NULL -- 'weekday','saturday','sunday'
                                                );
                                                CREATE INDEX ON schedules(station_id, line_id, day_type, arrival_time);

                                                CREATE TABLE fare_zones (
                                                  zone_id         INT PRIMARY KEY,
                                                    base_fare       INT NOT NULL,
                                                      per_km_rate     NUMERIC(6,2) NOT NULL,
                                                        transfer_discount NUMERIC(4,2) DEFAULT 0.10  -- 10% discount on transfer
                                                        );

                                                        -- Seoul fare data (as of 2024)
                                                        INSERT INTO fare_zones VALUES (1, 1550, 100.00, 0.10);

                                                        CREATE TABLE users (
                                                          id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                                                            home_station VARCHAR(8) REFERENCES stations(id),
                                                              saved_routes JSONB DEFAULT '[]',
                                                                tmoney_card  VARCHAR(32),
                                                                  pref_mode    VARCHAR(16) DEFAULT 'fastest',
                                                                    created_at   TIMESTAMPTZ DEFAULT NOW()
                                                                    );