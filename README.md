# Shell на Go

[![CI](https://github.com/Cuga77/codecrafters-shell-go/actions/workflows/ci.yml/badge.svg)](https://github.com/Cuga77/codecrafters-shell-go/actions/workflows/ci.yml)
[![progress-banner](https://backend.codecrafters.io/progress/shell/11c6de17-e7f4-4560-8100-f9ba7090bd25)](https://app.codecrafters.io/courses/shell/overview)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

POSIX-совместимая командная оболочка на Go, написанная в рамках челленджа
[«Build Your Own Shell»](https://app.codecrafters.io/courses/shell/overview) от CodeCrafters.

```console
$ echo "hello   world" 'single'
hello   world single
$ echo one two | wc -w
2
$ ls /nonexistent 2> err.txt
$ type echo
echo is a shell builtin
$ type ls
ls is /usr/bin/ls
```

## Возможности

- **Запуск программ.** Внешние команды ищутся по `PATH` и запускаются через `os/exec`.
- **Встроенные команды:** `echo`, `type`, `pwd`, `cd` (включая `~`), `history`, `exit`.
- **Разбор строки:** одинарные и двойные кавычки, экранирование обратным слэшем по правилам POSIX.
- **Конвейеры** `cmd1 | cmd2 | cmd3`, в которых могут участвовать и встроенные, и внешние команды.
- **Перенаправления:** `>`, `1>`, `>>`, `2>`, `2>>`.
- **История:** `history`, `history N`, `history -r/-w/-a <file>`. Если задан `HISTFILE`, история
  загружается при старте и сохраняется при выходе.
- **Автодополнение по Tab** для встроенных команд и исполняемых файлов из `PATH`: общий префикс
  подставляется сразу, при неоднозначности варианты выводятся по двойному Tab.

## Запуск

```bash
go run ./app
# или
./your_program.sh
```

Требуется Go 1.24+.
