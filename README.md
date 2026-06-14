# metro
transit routing app in Go

NATS subject map
train.position.{lineId}.{trainId}
Train → ETA workers
delay.event.{lineId}
ETA worker → cache invalidator
eta.update.{stationId}
ETA worker → WebSocket hub → clients
line.status.{lineId}
Cache invalidator → WebSocket hub → clients

![Metro Concept](public/images/1c.png)