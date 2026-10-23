// Drivers send their position every 4 s; one rider for every 10 drivers watches the 2 km around them.
// MAX=5000 k6 run k6/drivers.js
import { WebSocket } from 'k6/websockets';
import { Trend, Rate, Counter } from 'k6/metrics';
import exec from 'k6/execution';

const BASE = __ENV.BASE || 'ws://localhost:8088';
const MAX = +(__ENV.MAX || 5000);
const RAMP = __ENV.RAMP || '3m';
const C = { lat: 41.015, lng: 28.98, span: 0.05 };

const updateGap = new Trend('rider_update_gap', true); // time between two pushes to a rider, 1 s when healthy
const connectFail = new Rate('ws_connect_failed');
const driversSeen = new Trend('rider_drivers_seen');
const sent = new Counter('driver_updates_sent');

export const options = {
  scenarios: {
    drivers: { executor: 'ramping-vus', exec: 'driver', stages: [{ duration: RAMP, target: MAX }, { duration: '30s', target: MAX }] },
    riders: { executor: 'ramping-vus', exec: 'rider', stages: [{ duration: RAMP, target: Math.ceil(MAX / 10) }, { duration: '30s', target: Math.ceil(MAX / 10) }] },
  },
  summaryTrendStats: ['avg', 'p(50)', 'p(95)', 'max'],
};

const rnd = (c, s) => c - s + Math.random() * 2 * s;

export function driver() {
  const id = `d-${exec.vu.idInTest}`;
  let lat = rnd(C.lat, C.span), lng = rnd(C.lng, C.span);
  const ws = new WebSocket(`${BASE}/driver?id=${id}`);
  ws.onopen = () => {
    connectFail.add(false);
    const send = () => {
      if (ws.readyState !== 1) return;
      lat += (Math.random() - 0.5) * 0.0006; lng += (Math.random() - 0.5) * 0.0006;
      ws.send(JSON.stringify({ lat, lng })); sent.add(1);
    };
    send();
    const t = setInterval(send, 4000);
    ws.onclose = () => clearInterval(t);
  };
  ws.onerror = () => connectFail.add(true);
}

export function rider() {
  const lat = rnd(C.lat, C.span * 0.8), lng = rnd(C.lng, C.span * 0.8);
  const ws = new WebSocket(`${BASE}/rider?lat=${lat}&lng=${lng}&km=2`);
  let last = 0;
  ws.onopen = () => connectFail.add(false);
  ws.onmessage = (e) => {
    const now = Date.now();
    if (last) updateGap.add(now - last);
    last = now;
    driversSeen.add(JSON.parse(e.data).drivers.length);
  };
  ws.onerror = () => connectFail.add(true);
}
