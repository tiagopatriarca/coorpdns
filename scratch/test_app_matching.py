import sqlite3

db = sqlite3.connect('/docker/coorpdns/data/coorpdns.db')

rows = db.execute('''
    SELECT domain, SUM(total_queries) as total, SUM(blocked_queries) as blocked
    FROM daily_client_domain_stats
    WHERE date >= date('now', '-7 days')
    GROUP BY domain
    ORDER BY total DESC
''').fetchall()

apps = [
    {"id": "whatsapp", "name": "WhatsApp", "category": "messaging", "domains": ["whatsapp.com", "whatsapp.net"]},
    {"id": "telegram", "name": "Telegram", "category": "messaging", "domains": ["telegram.org", "t.me"]},
    {"id": "discord", "name": "Discord", "category": "messaging", "domains": ["discord.com", "discord.gg"]},
    {"id": "teams", "name": "Microsoft Teams", "category": "microsoft", "domains": ["teams.microsoft.com"]},
    {"id": "office_365", "name": "Office 365", "category": "microsoft", "domains": ["office.com"]},
    {"id": "azure", "name": "Azure", "category": "microsoft", "domains": ["azure.com"]},
    {"id": "edge", "name": "Edge", "category": "microsoft", "domains": ["microsoftedge.com", "edge.microsoft.com"]},
    {"id": "bing", "name": "Bing", "category": "microsoft", "domains": ["bing.com"]},
    {"id": "steam", "name": "Steam", "category": "games", "domains": ["steampowered.com", "steamserver.net"]},
    {"id": "epic_games", "name": "Epic Games", "category": "games", "domains": ["epicgames.com", "epicgames.dev"]},
    {"id": "roblox", "name": "Roblox", "category": "games", "domains": ["roblox.com"]},
    {"id": "spotify", "name": "Spotify", "category": "music", "domains": ["spotify.com", "scdn.co"]},
    {"id": "netflix", "name": "Netflix", "category": "streaming", "domains": ["netflix.com"]},
    {"id": "youtube", "name": "YouTube", "category": "streaming", "domains": ["youtube.com", "googlevideo.com"]},
    {"id": "itau", "name": "Itaú", "category": "bank", "domains": ["itau.com.br"]},
    {"id": "bradesco", "name": "Bradesco", "category": "bank", "domains": ["bradesco.com.br"]},
    {"id": "nubank", "name": "Nubank", "category": "bank", "domains": ["nubank.com.br"]},
    {"id": "chatgpt", "name": "ChatGPT", "category": "ai", "domains": ["chatgpt.com", "openai.com"]},
    {"id": "bet365", "name": "Bet365", "category": "betting", "domains": ["bet365.com"]},
    {"id": "betano", "name": "Betano", "category": "betting", "domains": ["betano.com"]},
    {"id": "blaze", "name": "Blaze", "category": "betting", "domains": ["blaze.com"]},
    {"id": "1xbet", "name": "1xBet", "category": "betting", "domains": ["1xbet.com"]},
]

app_stats = {}
for a in apps:
    app_stats[a["id"]] = {"id": a["id"], "name": a["name"], "category": a["category"], "queries": 0, "blocked": 0}

for domain, total, blocked in rows:
    d = domain.lower()
    for a in apps:
        matched = False
        for target in a["domains"]:
            if d == target or d.endswith("." + target):
                app_stats[a["id"]]["queries"] += total
                app_stats[a["id"]]["blocked"] += blocked
                matched = True
                break
        if matched:
            break

top_used = sorted([v for v in app_stats.values() if v["queries"] > 0], key=lambda x: x["queries"], reverse=True)
top_blocked = sorted([v for v in app_stats.values() if v["blocked"] > 0], key=lambda x: x["blocked"], reverse=True)

print("REAL TOP USED APPS:")
for u in top_used[:10]:
    print(u)

print("\nREAL TOP BLOCKED APPS:")
for b in top_blocked[:10]:
    print(b)
