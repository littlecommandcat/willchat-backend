import { useCallback, useEffect, useRef, useState } from "react";
import { api, onServerEvent, type Group, type Member, type Message } from "../lib/api";

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

export default function Chat() {
  const [currentUser, setCurrentUser] = useState<Member>({ id: "", name: "" });
  const [groups, setGroups] = useState<Group[]>([]);
  const [activeGroupId, setActiveGroupId] = useState<string | null>(null);
  const [messagesByGroup, setMessagesByGroup] = useState<Record<string, Message[]>>({});
  const [toast, setToast] = useState("");
  const [draft, setDraft] = useState("");
  const toastTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

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
    const data = await api.getState();
    setCurrentUser(data.member || { id: "", name: "" });
    setGroups(data.groups || []);
  }, []);

  const loadMessages = useCallback(
    async (groupId: string) => {
      const result = await api.getMessages(groupId);
      if (result.ok && result.messages) {
        setMessagesByGroup((prev) => ({ ...prev, [groupId]: result.messages! }));
      } else {
        setMessagesByGroup((prev) => ({ ...prev, [groupId]: [] }));
        if (result.error) {
          showToast(friendlyError(result as { error?: string; code?: string }, "載入訊息失敗"));
        }
      }
    },
    [showToast]
  );

  const selectGroup = useCallback(
    async (id: string) => {
      setActiveGroupId(id);
      const cached = messagesByGroup[id];
      if (!cached) {
        await loadMessages(id);
      }
    },
    [messagesByGroup, loadMessages]
  );

  const handleServerEvent = useCallback(
    async (evt: { type: string; [key: string]: unknown }) => {
      switch (evt.type) {
        case "ready":
          await applyState();
          break;
        case "message_send": {
          const gid = evt.group_id as string | undefined;
          const msg = evt.message as Message | undefined;
          if (gid && msg) {
            setMessagesByGroup((prev) => ({
              ...prev,
              [gid]: [...(prev[gid] || []), msg],
            }));
          }
          break;
        }
        case "message_delete": {
          const gid = evt.group_id as string | undefined;
          const msg = evt.message as Message | undefined;
          if (gid && msg) {
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
          const gid = evt.group_id as string | undefined;
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
          const gid = evt.group_id as string | undefined;
          const member = evt.member as Member | undefined;
          if (gid && member) {
            setGroups((prev) =>
              prev.map((g) =>
                g.id === gid && !g.members.some((m) => m.id === member.id)
                  ? { ...g, members: [...g.members, member] }
                  : g
              )
            );
          }
          break;
        }
        case "group_leave": {
          const gid = evt.group_id as string | undefined;
          const member = evt.member as Member | undefined;
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
          const member = evt.member as Member | undefined;
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
          setMessagesByGroup((prev) => {
            const next: Record<string, Message[]> = {};
            for (const [gid, msgs] of Object.entries(prev)) {
              next[gid] = msgs.map((m) =>
                m.author_id === member.id ? { ...m, author_name: member.name } : m
              );
            }
            return next;
          });
          break;
        }
        case "error":
          showToast((evt.message as string) || "連線發生錯誤");
          break;
        default:
          break;
      }
    },
    [applyState, showToast]
  );

  useEffect(() => {
    let unlisten: (() => void) | undefined;
    let cancelled = false;
    const init = async () => {
      const data = await api.getState();
      if (cancelled) return;
      setCurrentUser(data.member || { id: "", name: "" });
      setGroups(data.groups || []);
      unlisten = await onServerEvent(handleServerEvent);
    };
    init();
    return () => {
      cancelled = true;
      if (unlisten) unlisten();
    };
  }, [handleServerEvent]);

  const createGroup = async () => {
    const name = window.prompt("請輸入群組名稱：", "");
    if (name === null) return;
    const result = await api.createGroup(name.trim());
    if (!result.ok) {
      showToast(friendlyError(result as { error?: string; code?: string }, "建立群組失敗"));
    } else {
      await applyState();
    }
  };

  const joinGroup = async (groupId: string) => {
    const result = await api.joinGroup(groupId);
    if (!result.ok) {
      showToast(friendlyError(result as { error?: string; code?: string }, "加入群組失敗"));
    } else {
      await applyState();
    }
  };

  const leaveGroup = async (groupId: string) => {
    const result = await api.leaveGroup(groupId);
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
    if (!window.confirm("確定要刪除此群組？")) return;
    const result = await api.deleteGroup(groupId);
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
    if (!activeGroupId) return;
    const text = draft.trim();
    if (!text) return;
    setDraft("");
    const result = await api.sendMessage(activeGroupId, text);
    if (!result.ok) {
      showToast(friendlyError(result as { error?: string; code?: string }, "傳送失敗"));
    }
  };

  const activeGroup = findGroup(activeGroupId);
  const activeMessages = activeGroupId ? messagesByGroup[activeGroupId] || [] : [];


  return (
    <div className="layout">
      <div className={`toast ${toast ? "visible" : ""}`}>{toast}</div>
      <aside className="sidebar">
        <div className="sidebar-header">
          <span>群組</span>
          <button className="sidebar-action" title="新增群組" onClick={createGroup}>
            ＋
          </button>
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
                <div className="group-members">{group.members.length} 位成員</div>
                <div className="group-actions" onClick={(e) => e.stopPropagation()}>
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
              const own = msg.author_id === currentUser.id;
              const author = msg.author_name || "成員";
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
            onChange={(e) => setDraft(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") sendMessage();
            }}
          />
          <button disabled={!activeGroup} onClick={sendMessage}>
            傳送
          </button>
        </div>
      </main>
      <aside className="members-panel">
        <div className="members-header">成員</div>
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


