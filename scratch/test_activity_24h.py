import sqlite3
from datetime import datetime, timedelta

db = sqlite3.connect('/docker/coorpdns/data/coorpdns.db')
now = datetime.now()
cutoff = (now - timedelta(hours=24)).strftime('%Y-%m-%d %H:%M:%S')

buckets = {}
ordered_keys = []
for i in range(23, -1, -1):
    t = now - timedelta(hours=i)
    k = t.strftime('%Y-%m-%d %H:00')
    label = t.strftime('%H:00')
    buckets[k] = {'time': label, 'queries': 0, 'blocked': 0}
    ordered_keys.append(k)

rows = db.execute('''
    SELECT replace(substr(timestamp, 1, 13), 'T', ' ') || ':00' as hour_bucket,
           COUNT(*) as total,
           SUM(CASE WHEN status = 'blocked' THEN 1 ELSE 0 END) as blocked
    FROM dns_queries
    WHERE timestamp >= ?
    GROUP BY hour_bucket
''', (cutoff,)).fetchall()

for hr, tot, blk in rows:
    if hr in buckets:
        buckets[hr]['queries'] = tot
        buckets[hr]['blocked'] = blk or 0

results = [buckets[k] for k in ordered_keys]
print(f"Total buckets: {len(results)}")
print("Sample last 5 buckets:", results[-5:])
total_q = sum(r['queries'] for r in results)
print(f"Total queries in last 24h: {total_q}")
