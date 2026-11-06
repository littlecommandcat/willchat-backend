import { useCallback, useEffect, useRef, useState } from "react";
import { api, connectEvents, type ConnectionStatus, type Group, type Member, type Message, type ServerEvent } from "../lib/api";
import StatusIcon from "../StatusIcon";

const errorCodeMap: Record<string, string> = {
  owner_cannot_leave: "群主不能退出自己的群組",
  not_group_owner: "只有群主才能刪除群組",
  not_group_member: "你不是此群組的成員",
  group_not_found: "找不到該群組",
  group_limit_reached: "已達建立群組上限",
  email_already_registered: "此 Email 已註冊",
  invalid_credentials: "帳號或密碼錯誤",
  too_many_attachments: "最多只能附加 4 個檔案",
  attachment_too_large: "附件檔案過大（每個需 ≤5MB，總計 ≤25MB）",
  invalid_attachment: "附件的圖片格式不支援",
  reply_message_not_found: "回覆的訊息不存在",
  not_message_author: "只有訊息作者才能刪除",
  redis_unavailable: "伺服器暫時無法使用，請稍後再試",
  invalid_request: "要求內容不正確",
  not_allowed: "沒有權限執行此操作",
};

function friendlyError(data: { error?: string; code?: string } | null, fallback: string) {
  if (data && data.code && errorCodeMap[data.code]) return errorCodeMap[data.code];
  if (data && data.error) return data.error;
  return fallback;
}

function formatTime(ts: number): string {
  if (!ts) return "";
  const d = new Date(Number(ts));
  const hh = d.getHours().toString().padStart(2, "0");
  const mm = d.getMinutes().toString().padStart(2, "0");
  return `${hh}:${mm}`;
}

export default function Chat({ token, onLogout }: { token: string; onLogout: () => void }) {
  const [status, setStatus] = useState<ConnectionStatus>("closed");
  const [currentUser, setCurrentUser] = useState<Member>({ id: "", name: "" });
  const [groups, setGroups] = useState<Group[]>([]);
  const [activeGroupId, setActiveGroupId] = useState<string | null>(null);
  const [messagesByGroup, setMessagesByGroup] = useState<Record<string, Message[]>>({});
  const [toast, setToast] = useState("");
  const [draft, setDraft] = useState("");
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [profileName, setProfileName] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [settingsBusy, setSettingsBusy] = useState(false);
  const [actionBusy, setActionBusy] = useState(false);
  const toastTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const activeGroupRef = useRef<string | null>(null);
  const messageVersion = useRef<Record<string, number>>({});
  activeGroupRef.current = activeGroupId;

  const showToast = useCallback((text: string) => {
    setToast(text);
    if (toastTimer.current) clearTimeout(toastTimer.current);
    toastTimer.current = setTimeout(() => setToast(""), 3000);
  }, []);

  const findGroup = useCallback(
    (id: string | null): Group | null => groups.find((g) => g.id === id) || null,
    [groups]
  );

  const applyState = useCallback(async () => {
    const result = await api.getState();
    if (!result.ok) {
      if (result.code === "invalid_token" || result.code === "missing_token") onLogout();
      else showToast(friendlyError(result, "無法載入帳號資料"));
      return;
    }
    const { member } = result.data;
    const records = member.groups || [];
    setCurrentUser(member);
    setGroups((previous) => records.map((group) => ({
      id: group.group_id,
      name: group.name || group.group_id,
      owner_id: group.owner_id || "",
      members: [member, ...(previous.find((g) => g.id === group.group_id)?.members || [])
        .filter((m) => m.id !== member.id)],
    })));
    setActiveGroupId((id) => records.some((group) => group.group_id === id) ? id : null);
  }, [onLogout, showToast]);

  const rememberMembers = useCallback((groupId: string, members: Member[]) => {
    setGroups((previous) => previous.map((group) => group.id === groupId ? {
      ...group,
      members: [...group.members.filter((m) => !members.some((next) => next.id === m.id)), ...members],
    } : group));
  }, []);

  const loadMessages = useCallback(
    async (groupId: string) => {
      if (!groupId.trim()) return;
      const version = messageVersion.current[groupId] || 0;
      const result = await api.getMessages(groupId);
      if (!result.ok) { showToast(friendlyError(result, "載入訊息失敗")); return; }
      if ((messageVersion.current[groupId] || 0) !== version) return;
      const messages = [...(result.data.messages || [])].sort((a, b) => a.timestamp - b.timestamp);
      setMessagesByGroup((prev) => ({ ...prev, [groupId]: messages }));
      const ids = Array.from(new Set(messages.slice(-6).map((msg) => msg.authorid)));
      for (const id of ids) {
        if (activeGroupRef.current !== groupId) break;
        const author = await api.getMember(groupId, id);
        if (author.ok) rememberMembers(groupId, [author.data.member]);
      }
    },
    [rememberMembers, showToast]
  );

  const selectGroup = useCallback(
    async (id: string) => {
      setActiveGroupId(id);
      await loadMessages(id);
    },
    [loadMessages]
  );

  const saveMessage = useCallback((gid: string, msg: Message) => {
    messageVersion.current[gid] = (messageVersion.current[gid] || 0) + 1;
    setMessagesByGroup((prev) => ({
      ...prev,
      [gid]: [...(prev[gid] || []).filter((m) => m.id !== msg.id), msg]
        .sort((a, b) => a.timestamp - b.timestamp),
    }));
  }, []);

  const handleServerEvent = useCallback(
    async ({ event, payload }: ServerEvent) => {
      const gid = payload?.group?.group_id;
      const msg = payload?.message;
      const member = payload?.member;
      switch (event) {
        case "ready":
          await applyState();
          if (activeGroupRef.current) await loadMessages(activeGroupRef.current);
          break;
        case "message_send": {
          if (gid && msg) saveMessage(gid, msg);
          break;
        }
        case "message_delete": {
          if (gid && msg) {
            messageVersion.current[gid] = (messageVersion.current[gid] || 0) + 1;
            setMessagesByGroup((prev) => ({
              ...prev,
              [gid]: (prev[gid] || []).filter((m) => m.id !== msg.id),
            }));
          }
          break;
        }
        case "group_create":
          await applyState();
          break;
        case "group_delete": {
          setGroups((prev) => prev.filter((g) => g.id !== gid));
          setMessagesByGroup((prev) => {
            const next = { ...prev };
            if (gid) delete next[gid];
            return next;
          });
          setActiveGroupId((cur) => (cur === gid ? null : cur));
          break;
        }
        case "group_join": {
          if (gid && member) rememberMembers(gid, [member]);
          break;
        }
        case "group_leave": {
          if (gid && member) {
            setGroups((prev) =>
              prev.map((g) =>
                g.id === gid
                  ? { ...g, members: g.members.filter((m) => m.id !== member.id) }
                  : g
              )
            );
          }
          break;
        }
        case "member_update": {
          if (!member) break;
          setCurrentUser((cur) => (cur.id === member.id ? member : cur));
          setGroups((prev) =>
            prev.map((g) => ({
              ...g,
              members: g.members.map((m) =>
                m.id === member.id ? { ...m, name: member.name } : m
              ),
            }))
          );
          break;
        }
        case "error":
          break;
        case "auth_required":
          onLogout();
          break;
        default:
          break;
      }
    },
    [applyState, loadMessages, onLogout, rememberMembers, saveMessage]
  );

  useEffect(() => {
    void applyState();
    return connectEvents(token, handleServerEvent, setStatus);
  }, [applyState, handleServerEvent, token]);

  useEffect(() => () => {
    if (toastTimer.current) clearTimeout(toastTimer.current);
  }, []);

  const createGroup = async () => {
    if (actionBusy) return;
    const name = window.prompt("請輸入群組名稱：", "");
    if (name === null) return;
    if (name.trim().length < 1 || name.trim().length > 50) { showToast("群組名稱長度需為 1～50 字元"); return; }
    setActionBusy(true);
    let result;
    try { result = await api.createGroup(name.trim()); }
    catch { setActionBusy(false); showToast("建立群組失敗"); return; }
    setActionBusy(false);
    if (!result.ok) {
      showToast(friendlyError(result as { error?: string; code?: string }, "建立群組失敗"));
    } else {
      await applyState();
    }
  };

  const joinGroup = async (groupId: string) => {
    if (actionBusy || !groupId.trim()) return;
    setActionBusy(true);
    let result;
    try { result = await api.joinGroup(groupId.trim()); }
    catch { setActionBusy(false); showToast("加入群組失敗"); return; }
    setActionBusy(false);
    if (!result.ok) {
      showToast(friendlyError(result as { error?: string; code?: string }, "加入群組失敗"));
    } else {
      await applyState();
    }
  };
  const joinByPaste = async () => {
    const groupId = window.prompt("請貼上群組 ID：", "");
    if (groupId === null) return;
    await joinGroup(groupId.trim());
  };
  const copyGroupId = async (groupId: string) => {
    let copied = false;
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(groupId);
        copied = true;
      }
    } catch {
      copied = false;
    }
    if (!copied) {
      try {
        const ta = document.createElement("textarea");
        ta.value = groupId;
        ta.style.position = "fixed";
        ta.style.opacity = "0";
        document.body.appendChild(ta);
        ta.select();
        copied = document.execCommand("copy");
        document.body.removeChild(ta);
      } catch {
        copied = false;
      }
    }
    if (copied) {
      showToast("已複製群組 ID");
    } else {
      showToast("複製失敗，請手動複製");
    }
  };





  const leaveGroup = async (groupId: string) => {
    if (actionBusy || !groupId.trim() || !window.confirm("確定要退出此群組？")) return;
    setActionBusy(true);
    let result;
    try { result = await api.leaveGroup(groupId.trim()); }
    catch { setActionBusy(false); showToast("退出群組失敗"); return; }
    setActionBusy(false);
    if (!result.ok) {
      showToast(friendlyError(result as { error?: string; code?: string }, "退出群組失敗"));
    } else {
      setGroups((prev) => prev.filter((g) => g.id !== groupId));
      setMessagesByGroup((prev) => {
        const next = { ...prev };
        delete next[groupId];
        return next;
      });
      setActiveGroupId((cur) => (cur === groupId ? null : cur));
    }
  };

  const deleteGroup = async (groupId: string) => {
    if (actionBusy || !groupId.trim() || !window.confirm("確定要刪除此群組？")) return;
    setActionBusy(true);
    let result;
    try { result = await api.deleteGroup(groupId.trim()); }
    catch { setActionBusy(false); showToast("刪除群組失敗"); return; }
    setActionBusy(false);
    if (!result.ok) {
      showToast(friendlyError(result as { error?: string; code?: string }, "刪除群組失敗"));
    } else {
      setGroups((prev) => prev.filter((g) => g.id !== groupId));
      setMessagesByGroup((prev) => {
        const next = { ...prev };
        delete next[groupId];
        return next;
      });
      setActiveGroupId((cur) => (cur === groupId ? null : cur));
    }
  };

  const sendMessage = async () => {
    if (actionBusy || !activeGroupId) return;
    const text = draft.trim();
    if (!text) return;
    if (text.length > 2000) { showToast("訊息不可超過 2000 字元"); return; }
    setActionBusy(true);
    let result;
    try { result = await api.sendMessage(activeGroupId, text); }
    catch { setActionBusy(false); showToast("傳送訊息失敗"); return; }
    setActionBusy(false);
    if (!result.ok) {
      showToast(friendlyError(result as { error?: string; code?: string }, "傳送失敗"));
    } else {
      saveMessage(activeGroupId, result.data.message);
      setDraft((current) => current.trim() === text ? "" : current);
    }
  };

  const openSettings = () => {
    setProfileName(currentUser.name);
    setSettingsOpen(true);
  };
  const saveProfile = async () => {
    if (settingsBusy) return;
    if (profileName.trim().length < 3 || profileName.trim().length > 50) { showToast("使用者名稱需為 3～50 字元"); return; }
    setSettingsBusy(true);
    let result;
    try { result = await api.updateProfile(profileName.trim()); }
    catch { setSettingsBusy(false); showToast("更新個人資料失敗"); return; }
    setSettingsBusy(false);
    if (!result.ok) showToast(friendlyError(result, "更新個人資料失敗"));
    else { setCurrentUser((u) => ({ ...u, name: profileName.trim() })); showToast("個人資料已更新"); }
  };
  const resetPassword = async () => {
    if (settingsBusy) return;
    if (newPassword.length < 8) { showToast("新密碼至少需要 8 個字元"); return; }
    if (newPassword !== confirmPassword) { showToast("兩次密碼不一致"); return; }
    setSettingsBusy(true);
    let result;
    try { result = await api.resetPassword(newPassword); }
    catch { setSettingsBusy(false); showToast("重設密碼失敗"); return; }
    setSettingsBusy(false);
    if (!result.ok) showToast(friendlyError(result, "重設密碼失敗"));
    else onLogout();
  };

  const activeGroup = findGroup(activeGroupId);
  const activeMessages = activeGroupId ? messagesByGroup[activeGroupId] || [] : [];


  return (
    <div className="layout">
      <StatusIcon status={status} />
      <div className={`toast ${toast ? "visible" : ""}`}>{toast}</div>
      <aside className="sidebar">
        <div className="sidebar-header">
          <span>群組</span>
          <div className="sidebar-actions">
            <button className="sidebar-action" title="貼上 ID 加入群組" onClick={joinByPaste}>
              ⤵
            </button>
            <button className="sidebar-action" title="新增群組" onClick={createGroup}>
              ＋
            </button>
          </div>
        </div>
        <ul className="group-list">
          {groups.map((group) => {
            const isOwner = group.owner_id === currentUser.id;
            const isMember = group.members.some((m) => m.id === currentUser.id);
            return (
              <li
                key={group.id}
                className={`group-item ${activeGroupId === group.id ? "active" : ""}`}
                onClick={() => selectGroup(group.id)}
              >
                <div className="group-name">{group.name}</div>
                <div className="group-actions" onClick={(e) => e.stopPropagation()}>
                  <button onClick={() => copyGroupId(group.id)}>複製 ID</button>
                  {isMember && !isOwner && (
                    <button onClick={() => leaveGroup(group.id)}>退出</button>
                  )}
                  {!isMember && (
                    <button onClick={() => joinGroup(group.id)}>加入</button>
                  )}
                  {isOwner && (
                    <button onClick={() => deleteGroup(group.id)}>刪除</button>
                  )}
                </div>
              </li>
            );
          })}
        </ul>
        <div className="account-settings">
          <span>{currentUser.name || "帳號"}</span>
          <button className="sidebar-action" title="帳號設定" onClick={openSettings}>⚙</button>
          <button className="sidebar-action" onClick={onLogout}>登出</button>
        </div>
      </aside>
      <main className="chat">
        <div className="chat-header">
          {activeGroup ? activeGroup.name : "請選擇群組"}
        </div>
        <div className="messages">
          {activeMessages.length === 0 ? (
            <div className="message-empty">
              {activeGroup ? "尚無訊息" : "選擇左側群組開始聊天"}
            </div>
          ) : (
            activeMessages.map((msg) => {
              const own = msg.authorid === currentUser.id;
              const author = activeGroup?.members.find((m) => m.id === msg.authorid)?.name || "成員";
              return (
                <div key={msg.id} className={`message ${own ? "own" : "other"}`}>
                  <div className="meta">
                    {(own ? currentUser.name : author) + " · " + formatTime(msg.timestamp)}
                  </div>
                  {msg.reply_to?.content && (
                    <div className="reply-quote">
                      {(msg.reply_to.authorid === currentUser.id ? "回覆：" : "回覆 ") +
                        msg.reply_to.content}
                    </div>
                  )}
                  <div>{msg.content}</div>
                  {(msg.attachments || []).map((att, i) =>
                    att.data ? (
                      <img
                        key={i}
                        className="attachment"
                        src={`data:${att.mime_type || "image/jpeg"};base64,${att.data}`}
                        alt=""
                      />
                    ) : null
                  )}
                </div>
              );
            })
          )}
        </div>
        <div className="chat-input">
          <input
            type="text"
            placeholder="輸入訊息..."
            disabled={!activeGroup}
            value={draft}
            maxLength={2000}
            onChange={(e) => setDraft(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.nativeEvent.isComposing) sendMessage();
            }}
          />
          <button disabled={!activeGroup || actionBusy} onClick={sendMessage}>
            傳送
          </button>
        </div>
      </main>
      {settingsOpen && <div className="settings-backdrop" onClick={() => setSettingsOpen(false)}>
        <section className="settings-panel" onClick={(e) => e.stopPropagation()}>
          <h2>帳號設定</h2>
          <label>名稱<input value={profileName} onChange={(e) => setProfileName(e.target.value)} /></label>
          <button disabled={settingsBusy} onClick={saveProfile}>儲存個人資料</button>
          <hr />
          <label>新密碼<input type="password" autoComplete="new-password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} minLength={8} /></label>
          <label>確認新密碼<input type="password" autoComplete="new-password" value={confirmPassword} onChange={(e) => setConfirmPassword(e.target.value)} minLength={8} /></label>
          <button disabled={settingsBusy} onClick={resetPassword}>重設密碼並返回登入</button>
          <button className="settings-cancel" onClick={() => setSettingsOpen(false)}>取消</button>
        </section>
      </div>}
      <aside className="members-panel">
        <div className="members-header">已知成員</div>
        <ul className="member-list">
          {(activeGroup ? activeGroup.members : []).map((member) => (
            <li key={member.id} className="member-item">
              <span className="member-avatar">{(member.name || "?").charAt(0)}</span>
              <span>{member.name}</span>
            </li>
          ))}
        </ul>
      </aside>
    </div>
  );
}


