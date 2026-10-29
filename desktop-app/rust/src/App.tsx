import { useState } from "react";
import Login from "./pages/Login";
import Register from "./pages/Register";
import Chat from "./pages/Chat";

type Page = "login" | "register" | "chat";

function App() {
  const [page, setPage] = useState<Page>("login");
  const [registered, setRegistered] = useState(false);

  const handleLogin = () => {
    setPage("chat");
  };

  const handleNavigateRegister = () => {
    setPage("register");
  };

  const handleRegisterDone = (ok: boolean) => {
    if (ok) {
      setRegistered(true);
      setPage("login");
    } else {
      setPage("login");
    }
  };

  if (page === "register") {
    return <Register onDone={handleRegisterDone} />;
  }

  if (page === "chat") {
    return <Chat />;
  }

  return (
    <>
      {registered && (
        <div id="toast" className="toast visible" style={{ zIndex: 2000 }}>
          註冊成功
        </div>
      )}
      <Login onLogin={handleLogin} onNavigateRegister={handleNavigateRegister} />
    </>
  );
}

export default App;

