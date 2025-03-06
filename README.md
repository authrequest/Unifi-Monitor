[![ko-fi](https://ko-fi.com/img/githubbutton_sm.svg)](https://ko-fi.com/L3L418JUWC)

# Unifi New Product Monitor

Welcome to my **Unifi Monitor** script, a comprehensive and efficient solution for [Monitoring https://store.ui.com for NEW products].

## Table of Contents

- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Installation](#installation)
- [Usage](#usage)
- [Contributing](#contributing)
- [License](#license)
- 

## Getting Started

To get a local copy up and running, follow these simple steps.

### Prerequisites

Ensure you have the following installed:

- Go programming language

### Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/authrequest/Unifi-Monitor.git
   ```
2. Navigate to the project directory:
   ```bash
   cd Unifi-Monitor
   ```
3. Install dependencies:
   ```bash
   # Install any Go dependencies if applicable
   go mod tidy
   ```

## Usage

Before running the program, make sure to configure the Discord webhook URL. 

1. Environment variable `DISCORD_WEBHOOK_URL` set on the command line.
2. Config file `/etc/unifi-monitor.yml`, with the key `DISCORD_WEBHOOK_URL:`


Run the project:

```bash
go run all_products.go
```

or

```bash
go run *
```

## Contributing

Contributions are what make the open-source community such an amazing place to learn, inspire, and create. Any contributions you make are **greatly appreciated**.

## License

Distributed under the [GNU GPL v3.0](LICENSE). See `LICENSE` for more information.

---

Thank you for considering contributing to this project!
