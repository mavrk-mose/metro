package internal

import "github.com/jackc/pgx/v5/pgxpool"

// LoadSubwayGraph reads static data from Postgres once at startup
// and builds the in-memory gonum graph.  Not called per request.
func LoadSubwayGraph(ctx context.Context, pg *pgxpool.Pool) (*SubwayGraph, error) {
  stations, err := loadStations(ctx, pg)
    if err != nil {
	    return nil, fmt.Errorf("load stations: %w", err)
		  }

		    edges, err := loadEdges(ctx, pg)
			  if err != nil {
			      return nil, fmt.Errorf("load edges: %w", err)
				    }

					  schedules, err := loadSchedules(ctx, pg)
					    if err != nil {
						    return nil, fmt.Errorf("load schedules: %w", err)
							  }

							    g := BuildSubwayGraph(stations, edges)
								  g.schedules = schedules

								    log.Printf("graph loaded: %d stations, %d edges", len(stations), len(edges))
									  return g, nil
									  }

									  func loadStations(ctx context.Context, pg *pgxpool.Pool) ([]Station, error) {
									    rows, err := pg.Query(ctx, `
										    SELECT id, name_ko, name_en,
											           ST_Y(location) AS lat,
													              ST_X(location) AS lon,
																             is_transfer, zone_id
																			     FROM stations ORDER BY id`)
																				   if err != nil {
																				       return nil, err
																					     }
																						   defer rows.Close()

																						     var out []Station
																							   for rows.Next() {
																							       var s Station
																								       rows.Scan(&s.ID, &s.NameKo, &s.NameEn, &s.Lat, &s.Lon, &s.IsTransfer, &s.ZoneID)
																									       out = append(out, s)
																										     }
																											   return out, rows.Err()
																											   }

																											   func loadEdges(ctx context.Context, pg *pgxpool.Pool) ([]Edge, error) {
																											     rows, err := pg.Query(ctx, `
																												     SELECT from_id, to_id, line_id,
																													            travel_secs, is_transfer, transfer_secs
																																    FROM edges`)
																																	  if err != nil {
																																	      return nil, err
																																		    }
																																			  defer rows.Close()

																																			    var out []Edge
																																				  for rows.Next() {
																																				      var e Edge
																																					      rows.Scan(&e.From, &e.To, &e.Line,
																																						        &e.TravelSecs, &e.IsTransfer, &e.TransferSecs)
																																								    out = append(out, e)
																																									  }
																																									    return out, rows.Err()
																																										}

																																										func loadSchedules(ctx context.Context, pg *pgxpool.Pool) (ScheduleMap, error) {
																																										  rows, err := pg.Query(ctx, `
																																										      SELECT station_id, line_id, direction, arrival_time, day_type
																																											      FROM schedules ORDER BY station_id, arrival_time`)
																																												    if err != nil {
																																													    return nil, err
																																														  }
																																														    defer rows.Close()

																																															  sm := make(ScheduleMap)
																																															    for rows.Next() {
																																																    var stnID, lineID, dir, dayType string
																																																	    var t time.Time
																																																		    rows.Scan(&stnID, &lineID, &dir, &t, &dayType)
																																																			    key := fmt.Sprintf("%s:%s:%s", stnID, lineID, dayType)
																																																				    sm[key] = append(sm[key], ScheduleEntry{Direction: dir, Time: t})
																																																					  }
																																																					    return sm, rows.Err()
																																																						}