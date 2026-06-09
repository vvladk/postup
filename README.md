# Postup

[Українська](#postup) · [English](#postup-english)

Open-source self-hosted інструмент, який допомагає команді провести ретроспективу ефективно та не загубити важливу інформацію після її завершення.

Postup розгортається як єдиний бінарний файл без зовнішніх залежностей. Не потребує підписки, хмарного акаунта або Docker. Всі дані зберігаються локально.

---

## Зміст

- [Що це і навіщо](#що-це-і-навіщо)
- [Функціонал для учасника](#функціонал-для-учасника)
- [Функціонал для адміна](#функціонал-для-адміна)
- [Інсталяція](#інсталяція)
- [Скидання паролів](#скидання-паролів)
- [Мережеві налаштування](#мережеві-налаштування)
- [Про назву](#про-назву)

---

## Що це і навіщо

Postup — інструмент для команд, яким важлива приватність даних і незалежність від зовнішніх сервісів. Ретроспективи проводяться на власній інфраструктурі: жодні дані не передаються третім сторонам і не зберігаються в хмарі.

**Типовий сценарій використання:**
1. Організатор ретро запускає Postup на локальній машині
2. Розсилає посилання для приєднання до ретроспективи учасникам у командний чат
3. Команда проводить ретро в реальному часі: картки, голосування, таймер, action items
4. Після завершення сервер зупиняється. Всі дані зберігаються локально у файлі `retro.db`
5. За потреби результати ретро експортуються у Markdown або CSV для подальшого завантаження в Confluence, Google Sheets тощо

---

## Функціонал для учасника

- **Дошка в реальному часі** — зміни миттєво відображаються у всіх учасників
- **Картки** — додавання, редагування та видалення власних карток
- **Голосування** — розподіл голосів між картками в межах встановленого ліміту

---

## Функціонал для адміна

### Управління

- **Команди** — створення команд, додавання учасників
- **Користувачі** — створення облікових записів
- **Шаблони** — налаштування колонок дошки

### Ретроспективи

- **Створення ретро** — вибір команди, шаблону, дати, ліміту голосів, тривалості
- **Посилання для приєднання** — генерується автоматично
- **Таймер** — запуск, пауза, скидання прямо з дошки
- **Action items** — призначення відповідального і дедлайну для кожного пункту
- **Створення action item з картки** — будь-яку картку можна перетягнути до колонки Action Items
- **Архів** — перегляд завершених ретро, експорт у Markdown і CSV

### Налаштування

- **Порт** — зміна порту сервера (вступає в дію після рестарту)
- **Base URL** — зовнішня адреса для посилань на приєднання; визначається автоматично або задається вручну, наприклад при використанні ngrok або у випадку подвійного NAT

---

## Інсталяція

### 1. Завантажити бінарник

На сторінці [Releases](https://github.com/vvladk/postup/releases) доступні архіви для всіх платформ:

| Платформа | Файл |
|---|---|
| macOS Apple Silicon | `postup-macos-arm64.zip` |
| macOS Intel | `postup-macos-amd64.zip` |
| Windows | `postup-windows-amd64.zip` |
| Linux | `postup-linux-amd64.zip` |

Після розпакування архіву в папці `postup/` знаходиться єдиний бінарний файл.

### 2. Перший запуск

Запускати рекомендується з терміналу.

**macOS / Linux:**
```bash
./postup
```

**Windows:**
```
postup.exe
```

При першому запуску відкриється майстер ініціалізації:

```
Postup is not initialized.
Initialize now? [y/n]: y
Admin email: pm@company.com
First name (optional): Олексій
Last name (optional): Коваль
Password (min. 4 characters): ****
Confirm password: ****
Done! Starting server...
```

Після завершення ініціалізації сервер доступний за адресою `http://localhost:8080`.

Файл бази даних `retro.db` створюється автоматично в підпапці `db/` поряд з бінарником. Зберігайте ці файли разом — `retro.db` містить всі дані.

### 3. Наступні запуски

Після завершення ініціалізації сервер запускається одразу без додаткових кроків.

---

## Скидання паролів

### Пароль адміна

Для скидання паролю адміна необхідно зупинити сервер і запустити з флагом `--reset`:

```bash
./postup --reset
```

```
Resetting admin password (pm@company.com)
Password (min. 4 characters): ****
Confirm password: ****
Password updated.
```

Після завершення сервер запускається звично.

### Пароль користувача

Скидання паролю користувача виконується адміном через UI. В розділі **Користувачі** навпроти потрібного облікового запису необхідно натиснути кнопку скидання паролю — буде згенеровано посилання, яке потрібно передати користувачу.

---

## Мережеві налаштування

Для підключення учасників з інших пристроїв сервер повинен бути доступний ззовні. Спосіб налаштування залежить від мережевої ситуації організатора ретро.

---

### Варіант 1: Білий IP + port forwarding

Підходить якщо провайдер надає білий IP — статичний або динамічний. Процес налаштування в обох випадках однаковий. При динамічному IP адреса може змінюватись, проте Postup в режимі "авто" визначає її автоматично при кожному запуску.

**Кроки:**

1. Поточний зовнішній IP відображається в рядку стану адміна у верхній частині сторінки (**Invite URL**)
2. В адмін-панелі роутера (зазвичай `192.168.1.1`) знайдіть розділ **Port Forwarding** (може називатися Virtual Server, NAT, PAT) і додайте правило:
   - Зовнішній порт: порт з налаштувань Postup (за замовчуванням `8080`). Рекомендований діапазон — від `1024` до `65535`; порти нижче `1024` зарезервовані системою і можуть вимагати прав адміністратора
   - Внутрішній IP: локальна адреса машини з Postup
   - Внутрішній порт: порт з налаштувань Postup (за замовчуванням `8080`)
   - Протокол: TCP
3. Запустіть Postup. Посилання для приєднання міститиме правильний зовнішній IP.

---

### Port forwarding не працює — подвійний NAT

Якщо після налаштування port forwarding учасники не можуть підключитися — можливо, провайдер використовує подвійний NAT.

**Як перевірити:**

1. Відкрийте адмін-панель роутера і знайдіть WAN-адресу
2. Відкрийте [https://api.ipify.org](https://api.ipify.org)

Якщо ці дві адреси **різні** — роутер сам знаходиться за NAT провайдера. Це подвійний NAT.

**Що це означає:** між вами і інтернетом є ще один NAT-пристрій на стороні провайдера, яким ви не керуєте. Port forwarding на вашому роутері в такому випадку не працює.

**Що робити:**
- Звернутися до провайдера з проханням надати білий статичний IP
- Використати ngrok (див. нижче)

---

### Варіант 2: ngrok

ngrok — тунельний сервіс, який створює публічну URL що веде на локальний сервер. Працює за будь-яких мережевих умов, включно з подвійним NAT.

Встановлення та налаштування — згідно з [офіційною документацією ngrok](https://ngrok.com/docs).

**Запуск:**

```bash
ngrok http 8080
```

Після запуску ngrok відобразить публічну URL. Її необхідно вказати в Postup: **Settings → Base URL → вимкніть "авто" → вставте ngrok URL**.

> **Обмеження безкоштовного плану:** URL змінюється при кожному перезапуску ngrok. Для фіксованої URL потрібен платний план.

---

### Варіант 3: Cloudflare Tunnel

Cloudflare Tunnel — альтернатива ngrok без обмежень на кількість сесій. Встановлення та налаштування — згідно з [офіційною документацією Cloudflare](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/).

**Важливо для користувачів в Україні:** Cloudflare може бути недоступний через блокування на рівні провайдера або через те що провайдер сам використовує інфраструктуру Cloudflare, що створює конфлікт маршрутизації. Перед використанням рекомендується перевірити доступність `cloudflare.com` для всіх учасників команди. Якщо виникають проблеми — використовуйте ngrok.

---

## Про назву

«Поступ» — українське слово, що означає рух уперед, прогрес через спільні зусилля. В період війни воно набуло особливого звучання як символ незупинного руху вперед попри все.

Але слово має й інші відлуння:

- У баскетболі *post up* — зайняти позицію, тримати позицію
- В інтернет-культурі *to post* — опублікувати думку, зробити її видимою
- У повсякденній мові *post up* — розмістити, виставити на показ

Саме це і відбувається на ретроспективі — люди займають позиції, висловлюють думки, роблять проблеми видимими і рухаються вперед разом.

**Поступ. Прогрес. Разом.**

---

## Ліцензія

MIT

---

---

# Postup (English)

Open-source self-hosted tool that helps teams run retrospectives effectively and preserve important information after each session.

Postup is distributed as a single binary with no external dependencies. No subscription, cloud account, or Docker required. All data is stored locally.

---

## Table of Contents

- [What it is and why](#what-it-is-and-why)
- [Features for participants](#features-for-participants)
- [Features for admins](#features-for-admins)
- [Installation](#installation)
- [Password reset](#password-reset)
- [Network setup](#network-setup)
- [About the name](#about-the-name)

---

## What it is and why

Postup is a tool for teams that value data privacy and independence from external services. Retrospectives are hosted on your own infrastructure — no data is sent to third parties or stored in the cloud.

**Typical usage:**
1. The retro organizer starts Postup on their local machine
2. Shares the join link with participants in the team chat
3. The team runs the retro in real time: cards, voting, timer, action items
4. After the session the server is shut down. All data is stored locally in `retro.db`
5. If needed, retro results can be exported as Markdown or CSV for upload to Confluence, Google Sheets, etc.

---

## Features for participants

- **Real-time board** — changes appear instantly for all participants
- **Cards** — add, edit, and delete your own cards
- **Voting** — distribute votes across cards within the set limit

---

## Features for admins

### Management

- **Teams** — create teams, add members
- **Users** — create accounts
- **Templates** — configure board columns

### Retrospectives

- **Create retro** — choose team, template, date, vote limit, duration
- **Join link** — generated automatically
- **Timer** — start, pause, reset directly from the board
- **Action items** — assign owner and deadline for each item
- **Create action item from card** — any card can be dragged to the Action Items column
- **Archive** — view completed retros, export as Markdown or CSV

### Settings

- **Port** — change server port (takes effect after restart)
- **Base URL** — external address for join links; resolved automatically or set manually, e.g. when using ngrok or in case of double NAT

---

## Installation

### 1. Download the binary

Archives for all platforms are available on the [Releases](https://github.com/vvladk/postup/releases) page:

| Platform | File |
|---|---|
| macOS Apple Silicon | `postup-macos-arm64.zip` |
| macOS Intel | `postup-macos-amd64.zip` |
| Windows | `postup-windows-amd64.zip` |
| Linux | `postup-linux-amd64.zip` |

After extracting the archive, the `postup/` folder contains a single binary file.

### 2. First run

Running from the terminal is recommended.

**macOS / Linux:**
```bash
./postup
```

**Windows:**
```
postup.exe
```

On the first run, an initialization wizard will open:

```
Postup is not initialized.
Initialize now? [y/n]: y
Admin email: pm@company.com
First name (optional): Alex
Last name (optional): Koval
Password (min. 4 characters): ****
Confirm password: ****
Done! Starting server...
```

After initialization, the server is available at `http://localhost:8080`.

The database file `retro.db` is created automatically in the `db/` subdirectory next to the binary. Keep these files together — `retro.db` contains all data.

### 3. Subsequent runs

After initialization is complete, the server starts immediately with no additional steps.

---

## Password reset

### Admin password

To reset the admin password, stop the server and run with the `--reset` flag:

```bash
./postup --reset
```

```
Resetting admin password (pm@company.com)
Password (min. 4 characters): ****
Confirm password: ****
Password updated.
```

After completion, start the server normally.

### User password

User password reset is performed by the admin via the UI. In the **Users** section, click the reset button next to the relevant account — a link will be generated and should be sent to the user.

---

## Network setup

For participants on other devices to connect to the board, the server must be accessible from outside. The setup method depends on the organizer's network situation.

---

### Option 1: Public IP + port forwarding

Works if the ISP provides a public IP — static or dynamic. The setup process is the same in both cases. With a dynamic IP, the address may change, but Postup in "auto" mode resolves it automatically on each startup.

**Steps:**

1. The current external IP is displayed in the admin status bar at the top of the page (**Invite URL**)
2. In the router admin panel (usually `192.168.1.1`), find the **Port Forwarding** section (may also be called Virtual Server, NAT, or PAT) and add a rule:
   - External port: port from Postup settings (default `8080`). Recommended range: `1024` to `65535`; ports below `1024` are reserved by the system and may require administrator privileges
   - Internal IP: local address of the machine running Postup
   - Internal port: port from Postup settings (default `8080`)
   - Protocol: TCP
3. Start Postup. The join link will contain the correct external IP.

---

### Port forwarding doesn't work — double NAT

If participants still can't connect after setting up port forwarding, the ISP may be using double NAT.

**How to check:**

1. Open the router admin panel and find the WAN address
2. Open [https://api.ipify.org](https://api.ipify.org)

If these two addresses are **different** — the router itself is behind the ISP's NAT. This is double NAT.

**What this means:** there is another NAT device on the ISP's side that you don't control. Port forwarding on your router will not work in this case.

**What to do:**
- Contact your ISP and request a public static IP
- Use ngrok (see below)

---

### Option 2: ngrok

ngrok is a tunneling service that creates a public URL routing to your local server. Works under any network conditions, including double NAT.

Installation and setup — follow the [official ngrok documentation](https://ngrok.com/docs).

**Run:**

```bash
ngrok http 8080
```

After starting, ngrok will display a public URL. Set it in Postup: **Settings → Base URL → disable "auto" → paste the ngrok URL**.

> **Free plan limitation:** the URL changes on every ngrok restart. A paid plan is required for a fixed URL.

---

### Option 3: Cloudflare Tunnel

Cloudflare Tunnel is an alternative to ngrok with no session limits. Installation and setup — follow the [official Cloudflare documentation](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/).

**Note for users in Ukraine:** Cloudflare may be inaccessible due to ISP-level blocking or because the ISP itself uses Cloudflare infrastructure, causing a routing conflict. Before using, verify that `cloudflare.com` is reachable for all team members. If issues arise — use ngrok instead.

---

## About the name

"Postup" sounds like progress in Ukrainian — and it is. The word means moving forward, progress through collective effort. During the war it took on special resonance as a symbol of unstoppable movement forward despite everything.

But the word carries extra meaning in English too:

- In basketball: *post up* — to take a position, stand your ground
- In internet culture: *to post* — to publish your thoughts, make them visible
- In everyday use: *post up* — to put things up, to make them visible

All of which is exactly what happens in a retro — people take positions, post their thoughts, make issues visible, and move forward together.

**Postup. Progress. Together.**

---

## License

MIT
