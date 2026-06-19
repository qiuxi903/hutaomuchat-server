const { v4: uuidv4 } = require('uuid');
const fs = require('fs');
const path = require('path');

const GLOBAL_CHAT_ID = 'global-chat-room';
const DATA_FILE = path.join(__dirname, 'data.json');

// Fields to persist (exclude volatile data like atAllUsage which resets daily)
const PERSIST_FIELDS = [
  'users', 'friendships', 'friendRequests', 'chats', 'chatMembers',
  'messages', 'offlineQueue', 'recallQueue', 'moments', 'momentLikes',
  'momentComments', 'stickers', 'announcements', 'groupFileSettings',
  'groupUidCounter', 'groupJoinRequests', 'pushTokens'
];

// 纯内存数据库
const db = {
  users: [],
  friendships: [],
  friendRequests: [],
  chats: [],
  chatMembers: [],
  messages: [],
  offlineQueue: [], // { id, recipient_id, message: {...}, timestamp }
  recallQueue: [],  // { id, recipient_id, chatId, messageId, timestamp }
  moments: [],
  momentLikes: [],
  momentComments: [],
  stickers: [], // { id, user_id, url, created_at }
  announcements: [], // { id, chat_id, content, created_by, created_at, at_all }
  groupFileSettings: [], // { file_id, chat_id, permanent, created_at }
  atAllUsage: new Map(), // userId -> { date: 'YYYY-MM-DD', count: number }
  groupUidCounter: 0,
  groupJoinRequests: [], // { id, from_user_id, chat_id, message, status: 'PENDING'|'ACCEPTED'|'REJECTED', created_at }
  pushTokens: [], // { user_id, cid, platform, updated_at }

  // Check and increment @all daily usage. Returns { allowed, remaining }
  checkAtAllUsage(userId) {
    const today = new Date().toISOString().split('T')[0];
    let usage = this.atAllUsage.get(userId);
    if (!usage || usage.date !== today) {
      usage = { date: today, count: 0 };
      this.atAllUsage.set(userId, usage);
    }
    const maxDaily = 20;
    if (usage.count >= maxDaily) {
      return { allowed: false, remaining: 0 };
    }
    usage.count++;
    return { allowed: true, remaining: maxDaily - usage.count };
  },

  generateGroupUid() {
    return `htmq-${String(++this.groupUidCounter).padStart(3, '0')}`;
  },

  getRecommendedUsers(excludeUserId, excludeIds = []) {
    const allExcluded = new Set([excludeUserId, ...excludeIds]);
    const candidates = this.users.filter(u => u.recommendable !== false && !allExcluded.has(u.id));
    const shuffled = candidates.sort(() => Math.random() - 0.5);
    return shuffled.slice(0, 10);
  },

  // 用户操作
  createUser(user) {
    this.users.push(user);
    return user;
  },
  findUserByPhone(phone) {
    return this.users.find(u => u.phone === phone);
  },
  findUserById(id) {
    return this.users.find(u => u.id === id);
  },
  findUserByUid(uid) {
    return this.users.find(u => u.uid === uid);
  },
  updateUser(id, updates) {
    const idx = this.users.findIndex(u => u.id === id);
    if (idx >= 0) {
      this.users[idx] = { ...this.users[idx], ...updates };
      return this.users[idx];
    }
    return null;
  },

  // 好友关系
  addFriendship(userId, friendId) {
    this.friendships.push({ user_id: userId, friend_id: friendId, created_at: Date.now() });
  },
  getFriends(userId) {
    return this.friendships.filter(f => f.user_id === userId).map(f => f.friend_id);
  },
  areFriends(userId, friendId) {
    return this.friendships.some(f => f.user_id === userId && f.friend_id === friendId);
  },
  removeFriendship(userId, friendId) {
    this.friendships = this.friendships.filter(f =>
      !(f.user_id === userId && f.friend_id === friendId) &&
      !(f.user_id === friendId && f.friend_id === userId)
    );
  },

  // 聊天记录
  createChat(chat) {
    this.chats.push(chat);
    return chat;
  },
  getChat(chatId) {
    return this.chats.find(c => c.id === chatId);
  },
  addChatMember(chatId, userId, role = 'MEMBER') {
    this.chatMembers.push({ chat_id: chatId, user_id: userId, role });
  },
  getChatMembers(chatId) {
    return this.chatMembers.filter(cm => cm.chat_id === chatId).map(cm => cm.user_id);
  },
  removeChatById(chatId) {
    this.chats = this.chats.filter(c => c.id !== chatId);
    this.chatMembers = this.chatMembers.filter(cm => cm.chat_id !== chatId);
    this.messages = this.messages.filter(m => m.chat_id !== chatId);
  },

  // Remove a single member from a chat (for leave group)
  removeChatMember(chatId, userId) {
    this.chatMembers = this.chatMembers.filter(cm => !(cm.chat_id === chatId && cm.user_id === userId));
  },

  // 消息
  addMessage(msg) {
    this.messages.push(msg);
    return msg;
  },
  getMessages(chatId) {
    return this.messages.filter(m => m.chat_id === chatId).sort((a, b) => a.timestamp - b.timestamp);
  },

  // 朋友圈
  addMoment(moment) {
    this.moments.push(moment);
    return moment;
  },
  getMoments() {
    // 用户圈：只返回 scope=public 的动态
    return this.moments
      .filter(m => (m.scope || 'public') === 'public')
      .sort((a, b) => b.created_at - a.created_at);
  },
  addMomentLike(momentId, userId) {
    if (!this.momentLikes.some(l => l.moment_id === momentId && l.user_id === userId)) {
      this.momentLikes.push({ moment_id: momentId, user_id: userId });
    }
  },
  getMomentLikes(momentId) {
    return this.momentLikes.filter(l => l.moment_id === momentId);
  },
  addMomentComment(comment) {
    this.momentComments.push(comment);
    return comment;
  },
  getMomentComments(momentId) {
    return this.momentComments.filter(c => c.moment_id === momentId);
  },
  removeMomentLike(momentId, userId) {
    const idx = this.momentLikes.findIndex(l => l.moment_id === momentId && l.user_id === userId);
    if (idx >= 0) this.momentLikes.splice(idx, 1);
  },

  // 消息辅助
  findMessageById(id) {
    return this.messages.find(m => m.id === id);
  },
  updateMessageStatus(id, status) {
    const msg = this.messages.find(m => m.id === id);
    if (msg) { msg.status = status; return msg; }
    return null;
  },

  // Remove a message by ID (for recall)
  removeMessage(id) {
    const idx = this.messages.findIndex(m => m.id === id);
    if (idx >= 0) {
      this.messages.splice(idx, 1);
      return true;
    }
    return false;
  },

  // Offline message queue
  addToOfflineQueue(recipientId, message) {
    this.offlineQueue.push({
      id: require('crypto').randomUUID(),
      recipient_id: recipientId,
      message,
      timestamp: Date.now()
    });
  },

  // Get and clear pending messages for a user
  getPendingMessages(userId) {
    const pending = this.offlineQueue
      .filter(q => q.recipient_id === userId)
      .map(q => q.message)
      .sort((a, b) => (a.timestamp || 0) - (b.timestamp || 0));
    this.offlineQueue = this.offlineQueue.filter(q => q.recipient_id !== userId);
    return pending;
  },

  // Queue a recall event for an offline user
  addToRecallQueue(recipientId, chatId, messageId) {
    this.recallQueue.push({
      id: require('crypto').randomUUID(),
      recipient_id: recipientId,
      chatId,
      messageId,
      timestamp: Date.now()
    });
  },

  // Get and clear pending recalls for a user
  getPendingRecalls(userId) {
    const pending = this.recallQueue
      .filter(q => q.recipient_id === userId)
      .map(q => ({ chatId: q.chatId, messageId: q.messageId }));
    this.recallQueue = this.recallQueue.filter(q => q.recipient_id !== userId);
    return pending;
  },

  // Push token management (GeTui UniPush CID)
  setPushToken(userId, cid, platform = 'android') {
    const existing = this.pushTokens.find(t => t.user_id === userId);
    if (existing) {
      existing.cid = cid;
      existing.platform = platform;
      existing.updated_at = Date.now();
    } else {
      this.pushTokens.push({ user_id: userId, cid, platform, updated_at: Date.now() });
    }
  },

  getPushToken(userId) {
    const token = this.pushTokens.find(t => t.user_id === userId);
    return token ? token.cid : null;
  },

  removePushToken(userId) {
    this.pushTokens = this.pushTokens.filter(t => t.user_id !== userId);
  },

  // 删除朋友圈
  removeMoment(momentId, userId) {
    const idx = this.moments.findIndex(m => m.id === momentId && m.user_id === userId);
    if (idx >= 0) {
      this.moments.splice(idx, 1);
      // 同时删除关联的点赞和评论
      this.momentLikes = this.momentLikes.filter(l => l.moment_id !== momentId);
      this.momentComments = this.momentComments.filter(c => c.moment_id !== momentId);
      return true;
    }
    return false;
  },

  // ========== 群公告管理 ==========

  // Add announcement
  addAnnouncement(announcement) {
    this.announcements.push(announcement);
    return announcement;
  },

  // Get all announcements for a group (newest first)
  getAnnouncements(chatId) {
    return this.announcements
      .filter(a => a.chat_id === chatId)
      .sort((a, b) => b.created_at - a.created_at);
  },

  // Remove an announcement (by admin/owner)
  removeAnnouncement(announcementId, chatId) {
    const idx = this.announcements.findIndex(a => a.id === announcementId && a.chat_id === chatId);
    if (idx >= 0) {
      this.announcements.splice(idx, 1);
      return true;
    }
    return false;
  },

  // Update group avatar
  updateGroupAvatar(chatId, avatarUrl) {
    const chat = this.chats.find(c => c.id === chatId);
    if (chat) { chat.group_avatar = avatarUrl; chat.avatar_url = avatarUrl; return chat; }
    return null;
  },

  // Toggle mute for a user
  toggleGroupMute(chatId, userId) {
    const chat = this.chats.find(c => c.id === chatId);
    if (!chat) return null;
    if (!chat.muted_by) chat.muted_by = [];
    const idx = chat.muted_by.indexOf(userId);
    if (idx >= 0) chat.muted_by.splice(idx, 1);
    else chat.muted_by.push(userId);
    return chat;
  },

  // Toggle pin for a user
  toggleGroupPin(chatId, userId) {
    const chat = this.chats.find(c => c.id === chatId);
    if (!chat) return null;
    if (!chat.pinned_by) chat.pinned_by = [];
    const idx = chat.pinned_by.indexOf(userId);
    if (idx >= 0) chat.pinned_by.splice(idx, 1);
    else chat.pinned_by.push(userId);
    return chat;
  },

  // Add admin
  addGroupAdmin(chatId, userId) {
    const chat = this.chats.find(c => c.id === chatId);
    if (!chat) return null;
    if (!chat.admins) chat.admins = [];
    if (!chat.admins.includes(userId)) chat.admins.push(userId);
    return chat;
  },

  // Remove admin
  removeGroupAdmin(chatId, userId) {
    const chat = this.chats.find(c => c.id === chatId);
    if (!chat) return null;
    if (!chat.admins) chat.admins = [];
    chat.admins = chat.admins.filter(id => id !== userId);
    return chat;
  },

  // ========== 群文件管理 ==========

  // Get group files (messages of type FILE) with retention info
  getGroupFiles(chatId) {
    const now = Date.now();
    const sevenDays = 7 * 24 * 60 * 60 * 1000;
    return this.messages
      .filter(m => m.chat_id === chatId && m.type === 'FILE')
      .map(m => {
        const setting = this.groupFileSettings.find(s => s.file_id === m.id);
        const permanent = setting ? setting.permanent : false;
        const expired = !permanent && (now - m.timestamp > sevenDays);
        return { ...m, permanent, expired };
      })
      .filter(m => !m.expired);
  },

  // Mark a file as permanent
  markFilePermanent(fileId, chatId) {
    const existing = this.groupFileSettings.find(s => s.file_id === fileId && s.chat_id === chatId);
    if (existing) {
      existing.permanent = true;
      return existing;
    }
    const setting = { file_id: fileId, chat_id: chatId, permanent: true, created_at: Date.now() };
    this.groupFileSettings.push(setting);
    return setting;
  },

  // Remove a group file (admin delete)
  removeGroupFile(fileId, chatId) {
    const idx = this.messages.findIndex(m => m.id === fileId && m.chat_id === chatId && m.type === 'FILE');
    if (idx >= 0) {
      this.messages.splice(idx, 1);
      this.groupFileSettings = this.groupFileSettings.filter(s => s.file_id !== fileId);
      return true;
    }
    return false;
  },

  // ========== Stickers ==========
  // Get user's stickers
  getUserStickers(userId) {
    return this.stickers.filter(s => s.user_id === userId);
  },

  // Add sticker
  addSticker(userId, url) {
    const sticker = {
      id: require('crypto').randomUUID(),
      user_id: userId,
      url: url,
      created_at: Date.now()
    };
    this.stickers.push(sticker);
    return sticker;
  },

  // Delete sticker
  deleteSticker(stickerId, userId) {
    const idx = this.stickers.findIndex(s => s.id === stickerId && s.user_id === userId);
    if (idx >= 0) {
      this.stickers.splice(idx, 1);
      return true;
    }
    return false;
  },

  // ========== 全局聊天室 ==========

  // Get or create the global chat room
  getOrCreateGlobalChat() {
    let chat = this.chats.find(c => c.id === GLOBAL_CHAT_ID);
    if (!chat) {
      chat = {
        id: GLOBAL_CHAT_ID,
        type: 'GROUP',
        name: '用户圈',
        description: '所有用户的公共交流空间',
        announcement: '用户圈始终置顶在聊天列表中。长按可隐藏用户圈，在发现页面可重新打开。',
        avatar_url: '',
        group_avatar: '',
        muted_by: [],
        pinned_by: [],
        admins: [],
        owner_id: 'system',
        created_at: Date.now()
      };
      this.chats.push(chat);
    } else if (this.groupUidCounter === 0) {
      this.groupUidCounter = 1;
    }
    return chat;
  },

  // Auto-join a user to the global chat room
  autoJoinGlobalChat(userId) {
    const chat = this.getOrCreateGlobalChat();
    const members = this.getChatMembers(GLOBAL_CHAT_ID);
    if (!members.includes(userId)) {
      this.addChatMember(GLOBAL_CHAT_ID, userId, 'MEMBER');
    }
    return chat;
  },

  // ========== 好友朋友圈 ==========

  // Get moments for 朋友圈: user's own friends-scoped + friends' friends-scoped
  getFriendMoments(userId) {
    const friendIds = this.friendships
      .filter(f => (f.user_id === userId || f.friend_id === userId))
      .map(f => f.user_id === userId ? f.friend_id : f.user_id);
    return this.moments
      .filter(m => {
        const scope = m.scope || 'public';
        // User's own friends-scoped moments
        if (m.user_id === userId && scope === 'friends') return true;
        // Friends' friends-scoped moments only
        if (friendIds.includes(m.user_id) && scope === 'friends') return true;
        return false;
      })
      .sort((a, b) => b.created_at - a.created_at);
  },

  // ========== 群聊加入申请 ==========

  // Send join request
  sendJoinRequest(userId, chatId, message) {
      // Check not already a member
      const members = this.getChatMembers(chatId);
      if (members.includes(userId)) return { error: '已经是群成员' };
      // Check no pending request
      const existing = this.groupJoinRequests.find(r => r.from_user_id === userId && r.chat_id === chatId && r.status === 'PENDING');
      if (existing) return { error: '已发送过申请' };
      const request = {
          id: require('crypto').randomUUID(),
          from_user_id: userId,
          chat_id: chatId,
          message: message || '',
          status: 'PENDING',
          created_at: Date.now()
      };
      this.groupJoinRequests.push(request);
      return { request };
  },

  // Get pending join requests for a group
  getJoinRequests(chatId) {
      return this.groupJoinRequests.filter(r => r.chat_id === chatId && r.status === 'PENDING');
  },

  // Respond to join request
  respondToJoinRequest(requestId, chatId, status) {
      const idx = this.groupJoinRequests.findIndex(r => r.id === requestId && r.chat_id === chatId && r.status === 'PENDING');
      if (idx < 0) return null;
      this.groupJoinRequests[idx].status = status;
      if (status === 'ACCEPTED') {
          const userId = this.groupJoinRequests[idx].from_user_id;
          this.addChatMember(chatId, userId, 'MEMBER');
      }
      return this.groupJoinRequests[idx];
  },

  // Get all pending join requests for groups where user is owner/admin
  getManagedJoinRequests(userId) {
      // Find all groups where user is owner or admin
      const ownedOrAdminChats = this.chats.filter(c => 
          c.type === 'GROUP' && (c.owner_id === userId || (c.admins && c.admins.includes(userId)))
      );
      const chatIds = ownedOrAdminChats.map(c => c.id);
      return this.groupJoinRequests
          .filter(r => chatIds.includes(r.chat_id) && r.status === 'PENDING')
          .sort((a, b) => b.created_at - a.created_at);
  }
};

// ========== 持久化层 ==========

let saveTimer = null;
function scheduleSave() {
  if (saveTimer) clearTimeout(saveTimer);
  saveTimer = setTimeout(() => {
    try {
      const data = {};
      PERSIST_FIELDS.forEach(key => {
        if (key === 'atAllUsage') {
          // Convert Map to plain object
          data[key] = Object.fromEntries(db[key]);
        } else {
          data[key] = db[key];
        }
      });
      // Write to temp file first, then rename (atomic write)
      const tmpFile = DATA_FILE + '.tmp';
      fs.writeFileSync(tmpFile, JSON.stringify(data, null, 0));
      fs.renameSync(tmpFile, DATA_FILE);
    } catch (e) {
      console.error('Failed to save database:', e.message);
    }
  }, 1000); // Debounce 1 second
}

function loadFromDisk() {
  try {
    if (fs.existsSync(DATA_FILE)) {
      const raw = fs.readFileSync(DATA_FILE, 'utf8');
      const data = JSON.parse(raw);
      PERSIST_FIELDS.forEach(key => {
        if (data[key] !== undefined) {
          if (key === 'atAllUsage') {
            db[key] = new Map(Object.entries(data[key]));
          } else {
            db[key] = data[key];
          }
        }
      });
      console.log(`Database loaded from disk: ${db.users.length} users, ${db.chats.length} chats, ${db.messages.length} messages`);
      return true;
    }
  } catch (e) {
    console.error('Failed to load database:', e.message);
  }
  return false;
}

// Force save (for shutdown)
function saveNow() {
  if (saveTimer) clearTimeout(saveTimer);
  try {
    const data = {};
    PERSIST_FIELDS.forEach(key => {
      data[key] = db[key];
    });
    fs.writeFileSync(DATA_FILE, JSON.stringify(data, null, 0));
    console.log('Database saved on shutdown');
  } catch (e) {
    console.error('Failed to save on shutdown:', e.message);
  }
}

// Load saved data from disk
const loaded = loadFromDisk();

// Only initialize demo data if no saved data exists
if (!loaded) {
  db.getOrCreateGlobalChat();
  scheduleSave();
  console.log('Demo data initialized (global chat room only)');
}

// Save on process exit
process.on('SIGINT', () => { saveNow(); process.exit(0); });
process.on('SIGTERM', () => { saveNow(); process.exit(0); });
process.on('beforeExit', () => { saveNow(); });

// Safety-net: periodic save every 30s to catch any mutations that bypass the proxy
setInterval(() => { scheduleSave(); }, 30000);

// ========== Auto-save Proxy ==========
// Wraps db so that any property assignment or method call automatically triggers a save
const MUTATING_PREFIXES = ['create', 'add', 'remove', 'update', 'toggle', 'mark', 'delete', 'auto'];

const dbProxy = new Proxy(db, {
  set(target, prop, value) {
    const old = target[prop];
    target[prop] = value;
    if (PERSIST_FIELDS.includes(prop) && old !== value) {
      scheduleSave();
    }
    return true;
  },
  get(target, prop) {
    const value = target[prop];
    if (typeof value === 'function' && typeof prop === 'string') {
      const isMutating = MUTATING_PREFIXES.some(p => prop.startsWith(p));
      if (isMutating) {
        return function(...args) {
          const result = value.apply(target, args);
          scheduleSave();
          return result;
        };
      }
    }
    return value;
  }
});

dbProxy.GLOBAL_CHAT_ID = GLOBAL_CHAT_ID;
dbProxy.scheduleSave = scheduleSave;

module.exports = dbProxy;
