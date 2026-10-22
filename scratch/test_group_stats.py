import sqlite3

db = sqlite3.connect('/docker/coorpdns/data/coorpdns.db')
rows = db.execute('''
    SELECT g.id, g.name, g.color, COUNT(DISTINCT c.ip),
           COALESCE(SUM(d.total_queries), 0),
           COALESCE(SUM(d.blocked_queries), 0)
    FROM client_groups g
    LEFT JOIN clients c ON c.group_id = g.id
    LEFT JOIN daily_client_domain_stats d ON d.client_ip = c.ip AND d.date >= date('now', '-7 days')
    GROUP BY g.id
''').fetchall()

for r in rows:
    print(r)
