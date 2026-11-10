#!/bin/zsh
# Manage the fs助手 reimbursement service (launchd label it.mozm.fs-reimburse, Go build: ./fs-reimburse).
# Usage: fsctl.sh status|start|stop|restart|logs [n]|errors [n]|bindings|records <email> [n]
#        fsctl.sh upload <email> <MEAL|SHOPPING_RECEIPT> <memo|-> <image>...
#        fsctl.sh feishu recent [n] | fetch <message_id> <dir> | send-me <text> | send <email|mobile> <text> | remind "<YYYY-MM-DD HH:MM>" <text>
cd "$(dirname "$0")"
LABEL=it.mozm.fs-reimburse
PLIST=~/Library/LaunchAgents/$LABEL.plist
UID_=$(id -u)
case "$1" in
  status)  launchctl print gui/$UID_/$LABEL 2>/dev/null | grep -E '^\s*(state|pid|last exit code) =' || echo "not loaded" ;;
  start)   launchctl bootstrap gui/$UID_ $PLIST 2>/dev/null || launchctl kickstart gui/$UID_/$LABEL ;;
  stop)    launchctl bootout gui/$UID_/$LABEL ;;
  restart) launchctl kickstart -k gui/$UID_/$LABEL ;;
  logs)    tail -n ${2:-50} logs/out.log ;;
  errors)  tail -n ${2:-50} logs/err.log ;;
  bindings|records|upload|feishu)
    set -a; source ./.env; set +a
    case "$1" in
      bindings) ./fs-reimburse bindings ;;
      records)  shift; ./fs-reimburse records "$@" ;;
      upload)   shift; ./fs-reimburse upload "$@" ;;
      feishu)   shift; ./fs-reimburse feishu "$@" ;;
    esac ;;
  *) echo "usage: $0 status|start|stop|restart|logs [n]|errors [n]|bindings|records <email> [n]|upload <email> <type> <memo|-> <image>..."; exit 1 ;;
esac
