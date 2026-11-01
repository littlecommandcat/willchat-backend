import { useState } from "react";
import { api } from "../lib/api";

interface Props {
  onDone: (registered: boolean) => void;
}

export default function Register({ onDone }: Props) {
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (password !== confirm) {
      setMessage("驗證密碼不一致");
      return;
    }
    const isGmail = /^[a-zA-Z0-9._%+-]+@gmail.com$/.test(email.trim());
    if (username.trim().length < 3) {
      setMessage("帳號名稱過於簡短，請至少多於3字元");
      return;
    }
    if (!isGmail) {
      setMessage("限制Gmail進行註冊");
      return;
    }
    if (password.length < 8) {
      setMessage("密碼過於簡短，請至少多於8字元");
      return;
    }
    setBusy(true);
    setMessage("正在註冊...");
    const result = await api.register(username.trim(), email.trim(), password);
    setBusy(false);
    if (result.ok) {
      setMessage("註冊成功");
      onDone(true);
    } else {
      setMessage(result.error || "註冊失敗");
    }
  };

  return (
    <div className="auth-body">
      <form className="auth-form" onSubmit={handleSubmit}>
        <h2>建立帳號</h2>
        <label htmlFor="username">帳號</label>
        <input
          type="text"
          id="username"
          name="username"
          autoComplete="username"
          minLength={3}
          required
          value={username}
          onChange={(e) => setUsername(e.currentTarget.value)}
        />
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
          autoComplete="new-password"
          minLength={8}
          required
          value={password}
          onChange={(e) => setPassword(e.currentTarget.value)}
        />
        <label htmlFor="confirm_password">確認密碼</label>
        <input
          type="password"
          id="confirm_password"
          name="confirm_password"
          autoComplete="new-password"
          minLength={8}
          required
          value={confirm}
          onChange={(e) => setConfirm(e.currentTarget.value)}
        />
        <button type="submit" disabled={busy}>
          註冊
        </button>
        <p>
          已有帳號？
          <a href="#" onClick={(e) => { e.preventDefault(); onDone(false); }}>
            返回登入
          </a>
        </p>
        <p className="auth-message" role="status">{message}</p>
      </form>
    </div>
  );
}
