#!/usr/bin/env node

const fs = require('fs');
const path = require('path');

const SERVICE_SPECS = [
  {
    id: 'auth',
    title: 'Auth Service',
    baseUrlVar: 'auth_base_url',
    defaultBaseUrl: 'http://localhost:8001',
    files: ['cmd/auth-service/main.go'],
  },
  {
    id: 'queue',
    title: 'Queue Service',
    baseUrlVar: 'queue_base_url',
    defaultBaseUrl: 'http://localhost:8002',
    files: ['cmd/queue-service/main.go', 'internal/queue/docs.go'],
  },
  {
    id: 'booking',
    title: 'Booking Service',
    baseUrlVar: 'booking_base_url',
    defaultBaseUrl: 'http://localhost:8003',
    files: ['cmd/booking-service/main.go', 'internal/booking/docs.go'],
  },
  {
    id: 'websocket',
    title: 'WebSocket Hub Service',
    baseUrlVar: 'websocket_base_url',
    defaultBaseUrl: 'http://localhost:8004',
    files: ['cmd/websocket-hub/main.go'],
  },
  {
    id: 'printer',
    title: 'Printer Service',
    baseUrlVar: 'printer_base_url',
    defaultBaseUrl: 'http://localhost:8005',
    files: ['cmd/printer-service/main.go'],
  },
  {
    id: 'statistics',
    title: 'Statistics Service',
    baseUrlVar: 'statistics_base_url',
    defaultBaseUrl: 'http://localhost:8006',
    files: ['cmd/statistics-service/main.go'],
  },
  {
    id: 'public',
    title: 'Public Service',
    baseUrlVar: 'public_base_url',
    defaultBaseUrl: 'http://localhost:8007',
    files: ['cmd/public-service/main.go'],
  },
];

function normalizePath(p) {
  const out = String(p || '')
    .replace(/\/+/g, '/')
    .replace(/\/$/, '');
  return out || '/';
}

function joinPaths(base, rel) {
  if (!base) base = '';
  if (!rel) rel = '';
  const out = rel.startsWith('/') ? `${base}${rel}` : `${base}/${rel}`;
  return normalizePath(out);
}

function parseFileRoutes(absFile, serviceId) {
  const content = fs.readFileSync(absFile, 'utf8');
  const lines = content.split(/\r?\n/);

  // Track router/group variables and inherited auth.
  const vars = {
    r: { path: '', auth: false },
  };

  const routes = [];

  for (let i = 0; i < lines.length; i += 1) {
    const line = lines[i];

    // Example: api := r.Group("/api/v1", middleware.AuthRequired())
    let m = line.match(/^\s*(\w+)\s*:=\s*(\w+)\.Group\("([^"]*)"\s*(?:,\s*([^\)]*))?\)/);
    if (m) {
      const varName = m[1];
      const parentName = m[2];
      const rel = m[3];
      const extra = m[4] || '';
      const parent = vars[parentName] || { path: '', auth: false };
      vars[varName] = {
        path: joinPaths(parent.path, rel),
        auth: parent.auth || /AuthRequired\s*\(/.test(extra),
      };
      continue;
    }

    // Example: admin.Use(middleware.AuthRequired())
    m = line.match(/^\s*(\w+)\.Use\(([^\)]*)\)/);
    if (m) {
      const varName = m[1];
      const args = m[2] || '';
      if (vars[varName] && /AuthRequired\s*\(/.test(args)) {
        vars[varName].auth = true;
      }
      continue;
    }

    // Example: api.GET("/routes", h.ListRoutes)
    // Example: api.POST("/bookings", middleware.AuthRequired(), h.Create)
    m = line.match(/^\s*(\w+)\.(GET|POST|PUT|PATCH|DELETE|OPTIONS|Any)\("([^"]*)"(.*)$/);
    if (m) {
      const varName = m[1];
      const method = String(m[2]).toUpperCase() === 'ANY' ? 'ANY' : String(m[2]).toUpperCase();
      const rel = m[3];
      const tail = m[4] || '';

      const parent = vars[varName] || { path: '', auth: false };
      const routePath = joinPaths(parent.path, rel);
      const auth = parent.auth || /AuthRequired\s*\(/.test(tail);

      routes.push({
        service: serviceId,
        method,
        path: routePath,
        auth,
        sourceFile: absFile,
        sourceLine: i + 1,
      });
    }
  }

  return routes;
}

function uniqueRoutes(routes) {
  const seen = new Set();
  const out = [];

  for (const r of routes) {
    const key = `${r.service}|${r.method}|${r.path}`;
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(r);
  }

  return out;
}

function extractRoutes(repoRoot) {
  const all = [];

  for (const spec of SERVICE_SPECS) {
    for (const relFile of spec.files) {
      const absFile = path.resolve(repoRoot, relFile);
      if (!fs.existsSync(absFile)) continue;
      all.push(...parseFileRoutes(absFile, spec.id));
    }
  }

  return uniqueRoutes(all).sort((a, b) => {
    if (a.service !== b.service) return a.service.localeCompare(b.service);
    if (a.path !== b.path) return a.path.localeCompare(b.path);
    return a.method.localeCompare(b.method);
  });
}

module.exports = {
  SERVICE_SPECS,
  extractRoutes,
  normalizePath,
};
