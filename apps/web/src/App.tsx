import { useCallback, useEffect, useState } from "react";
import Login from "./pages/Login";
import Register from "./pages/Register";
import Chat from "./pages/Chat";
import { clearToken, getToken, setToken } from "./lib/cookies";

function App() {
  const [token, setSession] = useState(getToken);
  const [page, setPage] = useState<"login" | "register">("login");
  const [registered, setRegistered] = useState(false);

  const handleLogin = (token: string) => {
    setToken(token);
    setSession(token);
  };

  const logout = useCallback(() => {
    clearToken();
    setSession(undefined);
  }, []);

  useEffect(() => {
    if (!registered) return;
    const timer = setTimeout(() => setRegistered(false), 3000);
    return () => clearTimeout(timer);
  }, [registered]);

  if (token) return <Chat token={token} onLogout={logout} />;
  if (page === "register") {
    return <Register onDone={(ok) => { setRegistered(ok); setPage("login"); }} />;
  }

  return (
    <>
      {registered && (
        <div id="toast" className="toast visible" style={{ zIndex: 2000 }}>
          註冊成功
        </div>
      )}
      <Login onLogin={handleLogin} onNavigateRegister={() => setPage("register")} />
    </>
  );
}

export default App;
