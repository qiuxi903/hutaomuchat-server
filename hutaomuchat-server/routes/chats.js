const express = require('express');
const { v4: uuidv4 } = require('uuid');
const db = require('../db');
const { authMiddleware } = require('../middleware/auth');

const router = express.Router();

// 获取会话列表
router.get('/', authMiddleware, (req, res) => {
  const userChats = db.chats.filter(c => db.getChatMembers(c.id).includes(req.userId));
  const result = userChats.map(chat => {
    const members = db.getChatMembers(chat.id);
    let onlineStatus = 'OFFLINE';
    if (chat.type === 'SINGLE') {
      const otherId = members.find(m => m !== req.userId);
      if (otherId) {
        const other = db.findUserById(otherId);
        if (other) onlineStatus = other.online_status;
      }
    }
    return { ...chat, member_count: members.length, online_status: onlineStatus };
  });
  res.json({ chats: result });
});

// 创建单聊
router.post('/single', authMiddleware, (req, res) => {
  const { friend_id } = req.body;
  if (!friend_id) return res.status(400).json({ error: 'friend_id required' });

  const friend = db.findUserById(friend_id);
  if (!friend) return res.status(404).json({ error: 'Friend not found' });

  // Check if a single chat already exists between these two users
  const existingChat = db.chats.find(c => {
    if (c.type !== 'SINGLE') return false;
    const members = db.getChatMembers(c.id);
    return members.includes(req.userId) && members.includes(friend_id);
  });

  if (existingChat) {
    return res.json({ chat: existingChat });
  }

  const chatId = uuidv4();
  const chat = db.createChat({
    id: chatId,
    type: 'SINGLE',
    name: friend.nickname,
    avatar_url: '',
    description: '',
    owner_id: req.userId,
    created_at: Date.now()
  });

  db.addChatMember(chatId, req.userId, 'OWNER');
  db.addChatMember(chatId, friend_id);

  res.json({ chat });
});

// 创建群聊
router.post('/group', authMiddleware, (req, res) => {
  const { name, member_ids } = req.body;
  if (!name || !Array.isArray(member_ids) || member_ids.length === 0) {
    return res.status(400).json({ error: 'name and member_ids required' });
  }

  const chatId = uuidv4();
  const chat = db.createChat({
    id: chatId,
    type: 'GROUP',
    name,
    avatar_url: '',
    description: '',
    announcement: '',
    group_avatar: '',
    group_uid: db.generateGroupUid(),
    muted_by: [],
    pinned_by: [],
    admins: [req.userId],
    owner_id: req.userId,
    created_at: Date.now()
  });

  db.addChatMember(chatId, req.userId, 'OWNER');
  member_ids.forEach(mid => {
    if (mid !== req.userId) db.addChatMember(chatId, mid, 'MEMBER');
  });

  res.json({ chat });
});

// 获取全局聊天室
router.get('/global', authMiddleware, (req, res) => {
  db.getOrCreateGlobalChat();
  db.autoJoinGlobalChat(req.userId);
  res.json({ chatId: db.GLOBAL_CHAT_ID });
});

// GET /chats/search - Search groups by group_uid only (exact match)
router.get('/search', authMiddleware, (req, res) => {
  const { q } = req.query;
  if (!q) return res.status(400).json({ error: 'Query required' });
  const groups = db.chats.filter(c => {
    if (c.type !== 'GROUP') return false;
    if (c.id === 'global-chat-room') return false;
    // Only search by group_uid (exact match, case-insensitive)
    return c.group_uid && c.group_uid.toLowerCase() === q.toLowerCase();
  });
  // Enrich with member_count
  const result = groups.map(chat => {
    const members = db.getChatMembers(chat.id);
    return { ...chat, member_count: members.length };
  });
  res.json({ groups: result });
});

// GET /chats/managed-join-requests - Get all managed join requests (for contacts page)
router.get('/managed-join-requests', authMiddleware, (req, res) => {
    const requests = db.getManagedJoinRequests(req.userId);
    const enriched = requests.map(r => {
        const user = db.findUserById(r.from_user_id);
        const chat = db.getChat(r.chat_id);
        return {
            ...r,
            from_user: user ? { nickname: user.nickname, avatar_url: user.avatar_url, uid: user.uid } : null,
            chat_name: chat ? chat.name : ''
        };
    });
    res.json({ requests: enriched });
});

// POST /chats/:id/leave - Leave a group chat
router.post('/:id/leave', authMiddleware, (req, res) => {
  const chatId = req.params.id;
  // Cannot leave the global chat room
  if (chatId === db.GLOBAL_CHAT_ID) {
    return res.status(400).json({ error: '用户圈不能退出' });
  }
  const chat = db.getChat(chatId);
  if (!chat) return res.status(404).json({ error: 'Chat not found' });
  if (chat.type !== 'GROUP') return res.status(400).json({ error: '只能退出群聊' });
  const members = db.getChatMembers(chatId);
  if (!members.includes(req.userId)) {
    return res.status(400).json({ error: '你不是该群成员' });
  }
  // Remove user from chat members
  db.removeChatMember(chatId, req.userId);
  // If the leaving user was the owner, transfer ownership to the first admin or first member
  if (chat.owner_id === req.userId) {
    const remainingMembers = db.getChatMembers(chatId);
    if (remainingMembers.length > 0) {
      const admins = (chat.admins || []).filter(a => a !== req.userId && remainingMembers.includes(a));
      const newOwner = admins.length > 0 ? admins[0] : remainingMembers[0];
      chat.owner_id = newOwner;
      if (!chat.admins) chat.admins = [];
      if (!chat.admins.includes(newOwner)) chat.admins.push(newOwner);
    }
  }
  // Remove from admins list if was admin
  if (chat.admins) {
    chat.admins = chat.admins.filter(a => a !== req.userId);
  }
  // Notify remaining members via WebSocket
  if (db.wsManager) {
    const WebSocket = require('ws');
    const remainingMembers = db.getChatMembers(chatId);
    const user = db.findUserById(req.userId);
    remainingMembers.forEach(uid => {
      const client = db.wsManager.clients.get(uid);
      if (client && client.readyState === WebSocket.OPEN) {
        client.send(JSON.stringify({
          type: 'member_left',
          chatId: chatId,
          userId: req.userId,
          userName: user ? user.nickname : 'Unknown'
        }));
      }
    });
  }
  res.json({ success: true });
});

// 获取聊天信息
router.get('/:id', authMiddleware, (req, res) => {
  const members = db.getChatMembers(req.params.id);
  if (!members.includes(req.userId)) {
    return res.status(403).json({ error: 'Not a member' });
  }

  const chat = db.getChat(req.params.id);
  if (!chat) return res.status(404).json({ error: 'Chat not found' });

  res.json({ chat, members });
});

// PUT /chats/:id/announcement - Add new announcement (owner/admin only)
router.post('/:id/announcement', authMiddleware, (req, res) => {
  const { content, at_all } = req.body;
  const chat = db.chats.find(c => c.id === req.params.id);
  if (!chat) return res.status(404).json({ error: 'Chat not found' });

  // Check if user is owner or admin
  const isOwner = chat.owner_id === req.userId;
  const isAdmin = (chat.admins || []).includes(req.userId);
  if (!isOwner && !isAdmin) return res.status(403).json({ error: 'Only owner or admin can post announcements' });

  // Check @all rate limit
  let effectiveAtAll = at_all || false;
  if (effectiveAtAll) {
    const usage = db.checkAtAllUsage(req.userId);
    if (!usage.allowed) {
      effectiveAtAll = false;
      // Still create announcement but without @all
    }
  }

  const announcement = {
    id: uuidv4(),
    chat_id: req.params.id,
    content: content || '',
    created_by: req.userId,
    created_at: Date.now(),
    at_all: effectiveAtAll
  };

  db.addAnnouncement(announcement);

  // Also send as a card message to the chat
  const user = db.findUserById(req.userId);
  const msgId = uuidv4();
  const msgContent = JSON.stringify({
    type: 'ANNOUNCEMENT',
    content: content,
    at_all: effectiveAtAll,
    author: user ? user.nickname : 'Unknown'
  });
  const msg = {
    id: msgId,
    chat_id: req.params.id,
    sender_id: req.userId,
    sender_nickname: user ? user.nickname : 'Unknown',
    content: msgContent,
    type: 'CARD',
    status: 'SENT',
    timestamp: Date.now()
  };
  db.addMessage(msg);

  // Broadcast card message via WebSocket to all online chat members
  const WebSocket = require('ws');
  if (db.wsManager) {
    const memberIds = db.getChatMembers(req.params.id);
    memberIds.forEach(uid => {
      if (uid === req.userId) return;
      const client = db.wsManager.clients.get(uid);
      if (client && client.readyState === WebSocket.OPEN) {
        client.send(JSON.stringify({ type: 'message', message: msg }));
        // If @all, send special notification that bypasses DND
        if (effectiveAtAll) {
          client.send(JSON.stringify({
            type: 'at_all_notification',
            chatId: req.params.id,
            senderName: user ? user.nickname : 'Unknown',
            content: content,
            messageId: msgId
          }));
        }
      } else {
        db.addToOfflineQueue(uid, msg);
      }
    });
    // Update chat timestamp
    const chat = db.chats.find(c => c.id === req.params.id);
    if (chat) chat.updated_at = Date.now();
  }

  res.json({ announcement, message: msg });
});

// GET /chats/:id/announcements - Get all announcements
router.get('/:id/announcements', authMiddleware, (req, res) => {
  const announcements = db.getAnnouncements(req.params.id);
  // Enrich with author info
  const enriched = announcements.map(a => {
    const user = db.findUserById(a.created_by);
    return { ...a, author_name: user ? user.nickname : 'Unknown', author_avatar: user ? user.avatar_url : '' };
  });
  res.json({ announcements: enriched });
});

// DELETE /chats/:id/announcements/:announcementId
router.delete('/:id/announcements/:announcementId', authMiddleware, (req, res) => {
  const chat = db.chats.find(c => c.id === req.params.id);
  if (!chat) return res.status(404).json({ error: 'Chat not found' });
  const isOwner = chat.owner_id === req.userId;
  const isAdmin = (chat.admins || []).includes(req.userId);
  if (!isOwner && !isAdmin) return res.status(403).json({ error: 'Only owner or admin can delete announcements' });
  const removed = db.removeAnnouncement(req.params.announcementId, req.params.id);
  if (!removed) return res.status(404).json({ error: 'Announcement not found' });
  res.json({ success: true });
});

// PUT /chats/:id/avatar - Update group avatar
router.put('/:id/avatar', authMiddleware, (req, res) => {
  const { avatar_url } = req.body;
  console.log(`[Avatar] PUT /chats/${req.params.id}/avatar, url=${avatar_url}, userId=${req.userId}`);
  const chat = db.updateGroupAvatar(req.params.id, avatar_url || '');
  if (!chat) {
    console.log(`[Avatar] Chat not found: ${req.params.id}`);
    return res.status(404).json({ error: 'Chat not found' });
  }
  console.log(`[Avatar] Updated: chat=${chat.id}, group_avatar=${chat.group_avatar}`);
  res.json({ chat });
});

// POST /chats/:id/mute - Toggle mute
router.post('/:id/mute', authMiddleware, (req, res) => {
  const chat = db.toggleGroupMute(req.params.id, req.userId);
  if (!chat) return res.status(404).json({ error: 'Chat not found' });
  const isMuted = (chat.muted_by || []).includes(req.userId);
  res.json({ muted: isMuted });
});

// POST /chats/:id/pin - Toggle pin
router.post('/:id/pin', authMiddleware, (req, res) => {
  const chat = db.toggleGroupPin(req.params.id, req.userId);
  if (!chat) return res.status(404).json({ error: 'Chat not found' });
  const isPinned = (chat.pinned_by || []).includes(req.userId);
  res.json({ pinned: isPinned });
});

// POST /chats/:id/admins - Add admin (owner only)
router.post('/:id/admins', authMiddleware, (req, res) => {
  const { user_id } = req.body;
  const chat = db.chats.find(c => c.id === req.params.id);
  if (!chat || chat.owner_id !== req.userId) return res.status(403).json({ error: 'Only owner can add admins' });
  db.addGroupAdmin(req.params.id, user_id);
  res.json({ success: true });
});

// DELETE /chats/:id/admins/:userId - Remove admin
router.delete('/:id/admins/:userId', authMiddleware, (req, res) => {
  const chat = db.chats.find(c => c.id === req.params.id);
  if (!chat || chat.owner_id !== req.userId) return res.status(403).json({ error: 'Only owner can remove admins' });
  db.removeGroupAdmin(req.params.id, req.params.userId);
  res.json({ success: true });
});

// GET /chats/:id/files - Get group files
router.get('/:id/files', authMiddleware, (req, res) => {
  const files = db.getGroupFiles(req.params.id);
  res.json({ files });
});

// POST /chats/:id/files/:fileId/permanent - Mark file as permanent (owner/admin only)
router.post('/:id/files/:fileId/permanent', authMiddleware, (req, res) => {
  const chat = db.chats.find(c => c.id === req.params.id);
  if (!chat) return res.status(404).json({ error: 'Chat not found' });
  const isOwner = chat.owner_id === req.userId;
  const isAdmin = (chat.admins || []).includes(req.userId);
  if (!isOwner && !isAdmin) return res.status(403).json({ error: 'Only owner or admin can manage files' });
  db.markFilePermanent(req.params.fileId, req.params.id);
  res.json({ success: true });
});

// DELETE /chats/:id/files/:fileId - Delete group file (owner/admin only)
router.delete('/:id/files/:fileId', authMiddleware, (req, res) => {
  const chat = db.chats.find(c => c.id === req.params.id);
  if (!chat) return res.status(404).json({ error: 'Chat not found' });
  const isOwner = chat.owner_id === req.userId;
  const isAdmin = (chat.admins || []).includes(req.userId);
  if (!isOwner && !isAdmin) return res.status(403).json({ error: 'Only owner or admin can manage files' });
  const removed = db.removeGroupFile(req.params.fileId, req.params.id);
  if (!removed) return res.status(404).json({ error: 'File not found' });
  res.json({ success: true });
});

// ========== 群聊加入申请 ==========

// Send join request
router.post('/:id/join', authMiddleware, (req, res) => {
    const chat = db.getChat(req.params.id);
    if (!chat || chat.type !== 'GROUP') return res.status(404).json({ error: '群聊不存在' });
    if (chat.id === db.GLOBAL_CHAT_ID) return res.status(400).json({ error: '无法申请加入全局聊天室' });

    const { message } = req.body;
    const result = db.sendJoinRequest(req.userId, req.params.id, message);
    if (result.error) return res.status(400).json({ error: result.error });

    // Notify group owner/admins via WebSocket (best effort)
    const wss = req.app.get('wss');
    if (wss) {
        const admins = [chat.owner_id, ...(chat.admins || [])].filter(Boolean);
        admins.forEach(adminId => {
            const adminWs = wss.clients?.find(c => c.userId === adminId && c.readyState === 1);
            if (adminWs) {
                adminWs.send(JSON.stringify({
                    type: 'JOIN_REQUEST',
                    chatId: req.params.id,
                    chatName: chat.name,
                    from_user_id: req.userId,
                    requestId: result.request.id,
                    message: message || ''
                }));
            }
        });
    }

    res.json({ success: true, request: result.request });
});

// Get pending join requests for a group (owner/admin only)
router.get('/:id/join-requests', authMiddleware, (req, res) => {
    const chat = db.getChat(req.params.id);
    if (!chat) return res.status(404).json({ error: '群聊不存在' });
    if (chat.owner_id !== req.userId && !(chat.admins || []).includes(req.userId)) {
        return res.status(403).json({ error: '权限不足' });
    }

    const requests = db.getJoinRequests(req.params.id);
    // Enrich with user info
    const enriched = requests.map(r => {
        const user = db.findUserById(r.from_user_id);
        return { ...r, from_user: user ? { nickname: user.nickname, avatar_url: user.avatar_url, uid: user.uid } : null };
    });
    res.json({ requests: enriched });
});

// Respond to join request (owner/admin only)
router.post('/join-requests/:requestId/respond', authMiddleware, (req, res) => {
    const { status } = req.body;
    if (!['ACCEPTED', 'REJECTED'].includes(status)) return res.status(400).json({ error: '无效状态' });

    // Find the request to get the chat_id
    const allRequests = db.groupJoinRequests;
    const request = allRequests.find(r => r.id === req.params.requestId && r.status === 'PENDING');
    if (!request) return res.status(404).json({ error: '申请不存在或已处理' });

    // Verify permission
    const chat = db.getChat(request.chat_id);
    if (!chat) return res.status(404).json({ error: '群聊不存在' });
    if (chat.owner_id !== req.userId && !(chat.admins || []).includes(req.userId)) {
        return res.status(403).json({ error: '权限不足' });
    }

    const result = db.respondToJoinRequest(req.params.requestId, request.chat_id, status);
    if (!result) return res.status(404).json({ error: '处理失败' });

    // Notify the requester via WebSocket
    const wss = req.app.get('wss');
    if (wss) {
        const requesterWs = wss.clients?.find(c => c.userId === result.from_user_id && c.readyState === 1);
        if (requesterWs) {
            requesterWs.send(JSON.stringify({
                type: 'JOIN_REQUEST_RESPONSE',
                chatId: request.chat_id,
                chatName: chat.name,
                status: status,
                requestId: request.id
            }));
        }
    }

    res.json({ success: true, request: result });
});

module.exports = router;
