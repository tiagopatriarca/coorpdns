import sqlite3

db = sqlite3.connect('/docker/coorpdns/data/coorpdns.db')
top_domains = db.execute('''
    SELECT domain, SUM(total_queries) as total, SUM(blocked_queries) as blocked
    FROM daily_client_domain_stats
    WHERE date >= date('now', '-7 days')
    GROUP BY domain
    ORDER BY total DESC
    LIMIT 15
''').fetchall()

print("TOP 15 DOMAINS (Last 7 days):")
for td in top_domains:
    print(td)

top_blocked = db.execute('''
    SELECT domain, SUM(blocked_queries) as blocked
    FROM daily_client_domain_stats
    WHERE date >= date('now', '-7 days') AND blocked_queries > 0
    GROUP BY domain
    ORDER BY blocked DESC
    LIMIT 15
''').fetchall()

print("\nTOP 15 BLOCKED DOMAINS (Last 7 days):")
for tb in top_blocked:
    print(tb)
