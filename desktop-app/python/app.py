import logging
import sys

from PyQt6.QtCore import QUrl, Qt
from PyQt6.QtWidgets import QApplication, QMainWindow
from PyQt6.QtWebEngineWidgets import QWebEngineView
from PyQt6.QtWebChannel import QWebChannel

from bridge import Bridge
from utils import local_dir


def setup_logging():
    logging.basicConfig(
        level=logging.DEBUG,
        format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
        stream=sys.stdout,
    )


class MainWindow(QMainWindow):

    def __init__(self):
        super().__init__()

        self.setWindowTitle("WillChat")
        self.resize(1200, 800)
        
        self.browser = QWebEngineView()
        self.bridge = Bridge()
        self.bridge.start()
        self.channel = QWebChannel()
        self.channel.registerObject("bridge", self.bridge)
        self.browser.page().setWebChannel(self.channel)
        self.bridge.navigate_requested.connect(
            self.browser.setUrl
        )
        self.browser.setContextMenuPolicy(
            Qt.ContextMenuPolicy.NoContextMenu
        )
        local_path = str(local_dir / "index.html")
        self.browser.setUrl(QUrl.fromLocalFile(local_path))

        self.setCentralWidget(self.browser)

    def closeEvent(self, event):
        self.bridge.stop()
        super().closeEvent(event)


app = QApplication(sys.argv)
setup_logging()
window = MainWindow()
window.show()

sys.exit(app.exec())