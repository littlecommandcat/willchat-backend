import { useState } from "react";
import { api } from "../lib/api";

interface Props {
  onLogin: (member: { id: string; name: string }) => void;
  onNavigateRegister: () => void;
}

const errorCodeMap: Record<string, string> = {
  email_already_registered: "此 Email 已註冊",
  invalid_credentials: "帳號或密碼錯誤",
};

function friendlyError(data: { error?: string; code?: string }, fallback: string) {
  if (data.code && errorCodeMap[data.code]) return errorCodeMap[data.code];
  return data.error || fallback;
}

export default function Login({ onLogin, onNavigateRegister }: Props) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!email.trim() || !password) {
      setMessage("請輸入帳號與密碼");
      return;
    }
    const isGmail = /^[a-zA-Z0-9._%+-]+@gmail.com$/.test(email.trim());
    if (!isGmail || password.length < 8) {
      setMessage("帳號或密碼錯誤");
      return;
    }
    setBusy(true);
    setMessage("正在登入...");
    const result = await api.login(email.trim(), password);
    setBusy(false);
    if (result.ok) {
      setMessage("登入成功");
      const member = (result as { member?: { id: string; name: string } }).member || {
        id: "",
        name: "",
      };
      onLogin(member);
    } else {
      setMessage(friendlyError(result as { error?: string; code?: string }, "登入失敗"));
    }
  };

  // const handleRickroll = (e: React.MouseEvent) => {
  //   e.preventDefault();
  //   api.navigate("/rick");
  // };

  return (
    <div className="auth-body">
      <div id="toast" className="toast" />
      <form className="auth-form" onSubmit={handleSubmit}>
        <h2>WillChat</h2>
        <label htmlFor="email">電子郵件</label>
        <input
          type="email"
          id="email"
          name="email"
          autoComplete="email"
          required
          value={email}
          onChange={(e) => setEmail(e.currentTarget.value)}
        />
        <label htmlFor="password">密碼</label>
        <input
          type="password"
          id="password"
          name="password"
          autoComplete="current-password"
          required
          value={password}
          onChange={(e) => setPassword(e.currentTarget.value)}
        />
        <button type="submit" disabled={busy}>
          登入
        </button>
        <p>
          還沒有帳號？
          <a href="#" onClick={(e) => { e.preventDefault(); onNavigateRegister(); }}>
            立即註冊
          </a>
        </p>
        {/* <p>
          免費帳號？
          <a href="#" onClick={handleRickroll}>
            立即取得
          </a>
        </p> */}
        <p className="auth-message" role="status">{message}</p>
      </form>
    </div>
  );
}
