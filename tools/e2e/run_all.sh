#!/usr/bin/env bash
# Полный прогон живых проверок против стенда: трек Ж (ADMIN.md) и трек З (OWNER.md). Сначала
# API-уровень (python), потом браузер (playwright). Каждый скрипт идемпотентен и сам чистит за
# собой; порядок важен только тем, что браузерные наборы рассчитывают на чистый зал после API-наборов.
# Использование: tools/e2e/run_all.sh [all|e2e|pw|zh|z]   (по умолчанию all)
#   all — оба трека целиком; e2e / pw — только API / только браузер обоих треков;
#   zh — только трек Ж; z — только трек З (регрессия после каждой итерации трека З).
# Проверки трека З подхватываются сами: e2e_z<N>_*.py и pw_z<N>_*.js по номеру спринта
# (z2 раньше z10), первыми — проверки стенда e2e_z0_stand и pw_z0_stand.
# Окружение: E2E_API, E2E_DB, E2E_CHROME, E2E_OUT, E2E_SRV_LOG — см. README.md.
set -u
cd "$(dirname "$0")"
what=${1:-all}
case $what in all|e2e|pw|zh|z) ;; *) echo "группа: all | e2e | pw | zh | z"; exit 2 ;; esac
E2E=(e2e_incident_target e2e_booking_rules e2e_zh0_tail e2e_deposit_void e2e_zone_minutes e2e_club_day e2e_seat_offline e2e_zh5 e2e_zh6 e2e_zh8)
PW=(smoke_zh0 smoke_zh0c pw_zh1 pw_zh1_guest pw_zh2 pw_zh3 pw_zh4 pw_zh5 pw_zh6 pw_zh7 pw_zh8)
ZE2E=(e2e_z0_stand $(ls e2e_z[0-9]*.py 2>/dev/null | grep -vx 'e2e_z0_stand.py' | sort -V | sed 's/\.py$//'))
ZPW=(pw_z0_stand $(ls pw_z[0-9]*.js 2>/dev/null | grep -vx 'pw_z0_stand.js' | sort -V | sed 's/\.js$//'))
OUT=${E2E_OUT:-${TMPDIR:-/tmp}}
mkdir -p "$OUT"
fails=0
run() { # $1 — команда, $2 — имя
  pkill -f mock_agent.js 2>/dev/null; sleep 0.3   # хвосты мок-агентов от упавшего прогона
  local log="$OUT/$2.log"
  if timeout 600 $1 > "$log" 2>&1; then
    printf 'ok    %-22s %s\n' "$2" "$(grep -E '[Ии]того|[0-9]+ ok' "$log" | tail -1)"
  else
    fails=$((fails+1))
    printf 'FAIL  %-22s %s  (лог: %s)\n' "$2" "$(grep -E '^FAIL|Traceback|Error|CRASH' "$log" | head -3 | tr '\n' ' ')" "$log"
  fi
}
if [[ $what == all || $what == e2e || $what == zh ]]; then
  for t in "${E2E[@]}"; do run "python3 $t.py" "$t"; done
fi
if [[ $what == all || $what == e2e || $what == z ]]; then
  for t in "${ZE2E[@]}"; do run "python3 $t.py" "$t"; done
fi
if [[ $what == all || $what == pw || $what == zh ]]; then
  for t in "${PW[@]}"; do run "node $t.js" "$t"; done
fi
if [[ $what == all || $what == pw || $what == z ]]; then
  for t in "${ZPW[@]}"; do run "node $t.js" "$t"; done
fi
pkill -f mock_agent.js 2>/dev/null
echo; echo "провалов: $fails · логи и скриншоты: $OUT"
exit $((fails > 0))
