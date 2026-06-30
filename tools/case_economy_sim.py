#!/usr/bin/env python3
# Симуляция экономики кейсов — проверка алгоритма дропа из backend/internal/models/case.go.
# Запуск:  python3 tools/case_economy_sim.py
#
# Проверенный результат (500k открытий/тир, rtp=1.0):
#   тир      jackpot%  buster%  normal%   ~coins/кейс  ~бустер%/кейс
#   light     0.10%    5.02%    94.88%        169         0.050%
#   medium    0.49%   10.00%    89.51%        604         0.150%
#   heavy     1.01%   20.08%    78.91%       1492         0.402%
#   titan     2.98%   39.99%    57.03%       3198         1.200%
#   gods     10.01%   50.05%    39.94%      10005         2.502%
# Наблюдаемые шансы совпадают с конфигом; крайних случаев (паника rand) нет.

import random

# dropConfigs из case.go (chance из 100000, busterAmount в сотых процента)
cfgs = {
    'light':  dict(cmin=50,    cmax=200,   jchance=100,   jamt=50000, bchance=5000,  bamt=100),
    'medium': dict(cmin=200,   cmax=600,   jchance=500,   jamt=50000, bchance=10000, bamt=150),
    'heavy':  dict(cmin=500,   cmax=2000,  jchance=1000,  jamt=50000, bchance=20000, bamt=200),
    'titan':  dict(cmin=1000,  cmax=5000,  jchance=3000,  jamt=50000, bchance=40000, bamt=300),
    'gods':   dict(cmin=5000,  cmax=20000, jchance=10000, jamt=50000, bchance=50000, bamt=500),
}

def roll(c, rtp=1.0):
    r = random.randrange(100000)
    jt = int(c['jchance'] * rtp)
    bt = jt + int(c['bchance'] * rtp)
    if r < jt:
        return ('jackpot', c['jamt'])
    if r < bt:
        return ('buster', c['bamt'])
    return ('normal', c['cmin'] + random.randrange(c['cmax'] - c['cmin']))

def main(n=500_000):
    print(f"Симуляция: {n:,} открытий на тир, rtp=1.0\n")
    print(f"{'тир':7} {'jackpot%':>9} {'buster%':>9} {'normal%':>9} {'~coins':>9} {'~бустер%':>10}")
    for tier, c in cfgs.items():
        cj = cb = 0; coins = 0; bp = 0.0
        for _ in range(n):
            kind, amt = roll(c)
            if kind == 'jackpot': cj += 1; coins += amt
            elif kind == 'buster': cb += 1; bp += amt / 100.0
            else: coins += amt
        print(f"{tier:7} {cj/n*100:8.3f}% {cb/n*100:8.3f}% {(n-cj-cb)/n*100:8.2f}% {coins/n:8.0f} {bp/n:9.3f}%")

if __name__ == '__main__':
    main()
