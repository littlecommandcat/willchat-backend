# WillChat Desktop Python

透過之前所做的[willclient](../client/willclient/)開發一個python-desktop-app，PyQt6建立WebEngine與後端進行互動更新。

基礎的Python同步請求範例都已放在[example](../../examples/)可自行測試。

## Requirements

config.example.py -> config.py

```ini
pip install -r requirements.txt
```

## Configuration

```py
BASE_URL = "http://localhost:8080"
```

## Start

python >= 3.12

```ini
python app.py
```