correction: Vite 8.3.1 moved WebSocket addressing into server.ws; server.hmr remains relevant to overlay. Read actual client injection before writing proxy-attribution config.
correction: Vite port0 implementation itself contains a free-port-selection/bind interval and fails on collision; do not claim every port0 API necessarily retains the first reservation.
