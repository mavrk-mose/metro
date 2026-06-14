package config

import (
  "github.com/nats-io/nats.go"
    "github.com/nats-io/nats.go/encoders/protobuf"
	)

	func NewNATSConn(url string) (*nats.Conn, error) {
	  return nats.Connect(url,
	      nats.Name("seoul-metro-eta-service"),
		      nats.MaxReconnects(-1),          // reconnect forever
			      nats.ReconnectWait(2*time.Second),
				      nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
					        log.Printf("NATS disconnected: %v", err)
							    }),
								    nats.ReconnectHandler(func(nc *nats.Conn) {
									      log.Printf("NATS reconnected to %s", nc.ConnectedUrl())
									      }),
											      nats.ErrorHandler(func(nc *nats.Conn, sub *nats.Subscription, err error) {
												        log.Printf("NATS async error on %s: %v", sub.Subject, err)
														    }),
															  )
															  }

															  // PublishDelay is called when the ETA worker detects a train is running late
															  func PublishDelay(nc *nats.Conn, event DelayEvent) error {
															    subj := fmt.Sprintf("delay.event.%s", event.LineID)
																  data, err := json.Marshal(event)
																    if err != nil {
																	    return err
																		  }
																		    return nc.Publish(subj, data)
																			}

																			// PublishETA pushes a computed ETA update to WebSocket subscribers via NATS
																			func PublishETA(nc *nats.Conn, update ETAUpdateMsg) error {
																			  subj := fmt.Sprintf(SubjETAUpdate, update.StationID)
																			    data, _ := json.Marshal(update)
																				  return nc.Publish(subj, data)
																				  }