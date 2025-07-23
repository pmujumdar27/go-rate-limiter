# go-rate-limiter

## Self Notes

### TTL Buffer

Adding a TTL Buffer to expiration in redis protects the logic from clock drift, network latency. Also adds a safety margin.

### Gotchas

Go redis client converts float values to int before returning from lua script. So if you want to return a float from lua script, do a `tostring(value)` before returning. Learnt this the hard way.