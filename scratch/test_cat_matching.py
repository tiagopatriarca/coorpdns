import sqlite3

db = sqlite3.connect('/docker/coorpdns/data/coorpdns.db')

rows = db.execute('''
    SELECT domain, SUM(total_queries) as total, SUM(blocked_queries) as blocked
    FROM daily_client_domain_stats
    WHERE date >= date('now', '-7 days')
    GROUP BY domain
''').fetchall()

category_db = {
    "ads": ["doubleclick.net", "googleadservices.com", "googletagmanager.com", "scorecardresearch.com", "advertising.com", "ads.mozilla.org"],
    "trackers": ["telemetry.mozilla.org", "events.data.microsoft.com", "pipe.aria.microsoft.com", "amplitude.com", "google-analytics.com", "exp-tas.com"],
    "social": ["facebook.com", "instagram.com", "tiktok.com", "twitter.com", "x.com", "linkedin.com"],
    "gaming": ["steamserver.net", "steampowered.com", "epicgames.com", "epicgames.dev", "roblox.com", "discord.com", "discord.gg"],
    "streaming": ["youtube.com", "googlevideo.com", "netflix.com", "spotify.com", "deezer.com"],
    "messaging": ["whatsapp.com", "whatsapp.net", "telegram.org", "t.me", "zoom.us"],
    "microsoft": ["microsoft.com", "office.com", "azure.com", "bing.com", "msn.com"],
    "gambling": ["bet365.com", "betano.com", "blaze.com", "1xbet.com", "betfair.com"],
    "adult": ["xvideos.com", "pornhub.com", "xhamster.com", "onlyfans.com"]
}

cat_names = {
    "ads": "Anúncios & Publicidade",
    "trackers": "Rastreadores & Telemetria",
    "social": "Redes Sociais",
    "gaming": "Jogos & Games Online",
    "streaming": "Streaming de Vídeo & Música",
    "messaging": "Comunicação & Mensagens",
    "microsoft": "Serviços Microsoft & Nuvem",
    "gambling": "Apostas & Jogos de Azar",
    "adult": "Conteúdo Adulto"
}

cat_colors = {
    "ads": "#14b8a6",
    "trackers": "#6366f1",
    "social": "#3b82f6",
    "gaming": "#8b5cf6",
    "streaming": "#ec4899",
    "messaging": "#22c55e",
    "microsoft": "#0284c7",
    "gambling": "#f59e0b",
    "adult": "#ef4444"
}

stats = {k: {"id": k, "name": cat_names[k], "color": cat_colors[k], "queries": 0, "blocked": 0} for k in category_db}

for domain, total, blocked in rows:
    d = domain.lower()
    for cat, bases in category_db.items():
        matched = False
        for b in bases:
            if d == b or d.endswith("." + b):
                stats[cat]["queries"] += total
                stats[cat]["blocked"] += blocked
                matched = True
                break
        if matched:
            break

top_used = sorted([v for v in stats.values() if v["queries"] > 0], key=lambda x: x["queries"], reverse=True)
top_blocked = sorted([v for v in stats.values() if v["blocked"] > 0], key=lambda x: x["blocked"], reverse=True)

print("REAL TOP CATEGORIES:")
for u in top_used:
    print(u)

print("\nREAL TOP BLOCKED CATEGORIES:")
for b in top_blocked:
    print(b)
