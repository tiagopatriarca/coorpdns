import sqlite3
import json

db = sqlite3.connect('/docker/coorpdns/data/coorpdns.db')
rows = db.execute('''
    SELECT replace(substr(timestamp, 1, 13), 'T', ' ') || ':00' as hour_bucket,
           COUNT(*) as total,
           SUM(CASE WHEN status = 'blocked' THEN 1 ELSE 0 END) as blocked
    FROM dns_queries
    GROUP BY hour_bucket
    ORDER BY hour_bucket DESC
    LIMIT 24
''').fetchall()

print("HOURLY STATS (Real DB):")
for r in rows:
    print(r)

daily = db.execute('''
    SELECT date, SUM(total_queries), SUM(blocked_queries)
    FROM daily_client_domain_stats
    GROUP BY date
    ORDER BY date DESC
    LIMIT 7
''').fetchall()

print("\nDAILY STATS (Real DB):")
for d in daily:
    print(d)
