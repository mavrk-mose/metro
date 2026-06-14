package internal

// CacheRoute stores the computed route AND registers it in the line index
// so delay events know which keys to bust
func CacheRoute(ctx context.Context, rdb *redis.Client, from, to, pref string, route Route) error {
  key := fmt.Sprintf("route:%s:%s:%s", from, to, pref)

    pipe := rdb.Pipeline()

	  // Store the route result
	    data, _ := json.Marshal(route)
		  pipe.Set(ctx, key, data, 5*time.Minute)

		    // Store metadata for invalidation lookup
			  stationIDs := extractStationIDs(route)
			    stationsJSON, _ := json.Marshal(stationIDs)
				  pipe.HSet(ctx, key+":meta", "stations", string(stationsJSON))
				    pipe.Expire(ctx, key+":meta", 5*time.Minute)

					  // Register in per-line reverse index
					    for _, lineID := range extractLineIDs(route) {
						    idxKey := fmt.Sprintf("routeidx:line:%s", lineID)
							    pipe.SAdd(ctx, idxKey, key)
								    pipe.Expire(ctx, idxKey, 10*time.Minute)
									  }

									    _, err := pipe.Exec(ctx)
										  return err
										  }

										  func GetCachedRoute(ctx context.Context, rdb *redis.Client, from, to, pref string) (*Route, error) {
										    key := fmt.Sprintf("route:%s:%s:%s", from, to, pref)
											  data, err := rdb.Get(ctx, key).Bytes()
											    if err == redis.Nil {
												    return nil, nil // cache miss
													  }
													    if err != nil {
														    return nil, err
															  }
															    var route Route
																  return &route, json.Unmarshal(data, &route)
																  }