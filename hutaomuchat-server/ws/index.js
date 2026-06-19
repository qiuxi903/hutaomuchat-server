const WebSocket = require('ws');
const jwt = require('jsonwebtoken');
const { v4: uuidv4 } = require('uuid');
const db = require('../db');
const logger = require('../logger');
const { JWT_SECRET } = require('../middleware/auth');
const getui = require('../getui');

class WebSocketManager {
  constructor(server) {
    this.wss = new WebSocket.Server({ server });
    this.clients = new Map(); // userId -> BCM (broadcast channel)
    this.getui = getui;
    this.init();
  }

  // Check if user is admin or owner of a chat
  isAdminOrOwner(userId, chatId) {
    const chat = db.getChat(chatId);
    if (!chat) return false;
    return chat.owner_id === userId || (chat.admins || []).includes(userId);
  }

  // Force disconnect a user (e.g., when banned by admin)
  disconnectUser(userId) {
    const ws = this.clients.get(userId);
    if (ws) {
      ws.send(JSON.stringify({ type: 'force_disconnect', reason: 'banned' }));
      ws.close(4003, 'banned');
      this.clients.delete(userId);
      this.broadcastOnlineStatus(userId, 'OFFLINE');
      return true;
    }
    return false;
  }

  init() {
    this.wss.on('connection', (ws) => {
      console.log('New WebSocket connection');
      ws.isAlive = true;

      ws.on('pong', () => { ws.isAlive = true; });

      ws.on('message', (data) => {
        try {
          const msg = JSON.parse(data);
          this.handleMessage(ws, msg);
        } catch (err) {
          ws.send(JSON.stringify({ type: 'error', message: 'Invalid JSON' }));
        }
      });

      ws.on('close', () => {
        if (ws.userId) {
          this.clients.delete(ws.userId);
          this.broadcastOnlineStatus(ws.userId, 'OFFLINE');
        }
      });
    });

    // Heartbeat
    setInterval(() => {
      this.wss.clients.forEach((ws) => {
        if (!ws.isAlive) return ws.terminate();
        ws.isAlive = false;
        ws.ping();
      });
    }, 30000);
  }

  handleMessage(ws, msg) {
    switch (msg.type) {
      case 'authenticate':
        this.authenticate(ws, msg.token);
        break;
      case 'message':
        this.handleChatMessage(ws, msg);
        break;
      case 'RECALL':
        this.handleRecall(ws, msg);
        break;
      case 'typing':
        this.handleTyping(ws, msg);
        break;
      case 'message_ack':
        this.handleMessageAck(ws, msg);
        break;
      case 'friend_request':
        this.handleFriendRequest(ws, msg);
        break;
      default:
        ws.send(JSON.stringify({ type: 'error', message: 'Unknown message type' }));
    }
  }

  authenticate(ws, token) {
    try {
      const decoded = jwt.verify(token, JWT_SECRET);

      // Check if user is banned
      const user = db.findUserById(decoded.userId);
      if (user && user.banned) {
        ws.send(JSON.stringify({ type: 'error', message: '账号已被封禁' }));
        ws.close(4003, 'banned');
        return;
      }

      ws.userId = decoded.userId;
      ws.uid = decoded.uid;
      this.clients.set(decoded.userId, ws);
      ws.send(JSON.stringify({ type: 'authenticated', userId: decoded.userId }));
      this.broadcastOnlineStatus(decoded.userId, 'ONLINE');

      // Deliver any pending offline messages
      const pending = db.getPendingMessages(decoded.userId);
      if (pending.length > 0) {
        ws.send(JSON.stringify({
          type: 'pending_messages',
          messages: pending
        }));
        console.log(`Delivered ${pending.length} pending messages to user ${decoded.userId}`);
      }

      // Deliver any pending recall events
      const pendingRecalls = db.getPendingRecalls(decoded.userId);
      if (pendingRecalls.length > 0) {
        pendingRecalls.forEach(recall => {
          ws.send(JSON.stringify({
            type: 'RECALL',
            chatId: recall.chatId,
            messageId: recall.messageId
          }));
        });
        console.log(`Delivered ${pendingRecalls.length} pending recalls to user ${decoded.userId}`);
      }
    } catch (err) {
      ws.send(JSON.stringify({ type: 'auth_error', message: 'Invalid token' }));
    }
  }

  handleChatMessage(ws, msg) {
    if (!ws.userId) return;

    const { chatId, content, msgType } = msg;
    const messageType = msgType || 'TEXT';
    const messageId = msg.messageId || uuidv4();
    const timestamp = Date.now();

    // Update chat's updated_at (chat metadata only, no message storage)
    const chat = db.getChat(chatId);
    if (chat) chat.updated_at = timestamp;

    // Check for @all mentions
    let atAll = false;
    let atAllRemaining;
    let processedContent = content;
    if (messageType === 'TEXT' && content && (content.includes('@所有人') || content.includes('@全体成员'))) {
      if (!this.isAdminOrOwner(ws.userId, chatId)) {
        // Not admin/owner: strip @all mentions and send error
        processedContent = content.replace(/@所有人/g, '').replace(/@全体成员/g, '').trim();
        ws.send(JSON.stringify({ type: 'at_all_error', message: '只有管理员和群主才能@所有人' }));
      } else {
        const usage = db.checkAtAllUsage(ws.userId);
        if (!usage.allowed) {
          processedContent = content.replace(/@所有人/g, '').replace(/@全体成员/g, '').trim();
          ws.send(JSON.stringify({ type: 'at_all_error', message: '今日@所有人次数已用完（每天最多20次，剩余0次）' }));
        } else {
          atAll = true;
          atAllRemaining = usage.remaining;
        }
      }
    }

    // Get sender info
    const sender = db.findUserById(ws.userId);

    const messageData = {
      id: messageId,
      chat_id: chatId,
      sender_id: ws.userId,
      sender_nickname: sender ? sender.nickname : 'Unknown',
      content: processedContent,
      type: messageType,
      status: 'SENT',
      timestamp,
      at_all: atAll,
      at_all_remaining: atAll ? atAllRemaining : undefined
    };

    // Store message in db for history
    db.addMessage(messageData);

    // Log message send
    logger.user.sendMessage(ws.userId, chatId, messageType);

    const broadcastMsg = {
      type: 'message',
      message: messageData,
      chat_name: chat ? chat.name : '',
      is_group: chat ? chat.type === 'GROUP' : false
    };

    // Forward to all OTHER members of the chat (not back to sender)
    const memberIds = db.getChatMembers(chatId);
    memberIds.forEach(uid => {
      if (uid === ws.userId) return; // Don't echo back to sender
      const client = this.clients.get(uid);
      if (client && client.readyState === WebSocket.OPEN) {
        client.send(JSON.stringify(broadcastMsg));
        // If @all, also send a special notification that bypasses DND
        if (atAll) {
          client.send(JSON.stringify({
            type: 'at_all_notification',
            chatId,
            senderName: sender ? sender.nickname : 'Unknown',
            content: processedContent,
            messageId
          }));
        }
      } else {
        // Queue message for offline user
        db.addToOfflineQueue(uid, messageData);

        // Send push notification via GeTui UniPush (system-level push channel)
        const pushCid = db.getPushToken(uid);
        if (pushCid && this.getui && this.getui.isAvailable()) {
          const chatName = chat ? chat.name : '新消息';
          const isGroup = chat ? chat.type === 'GROUP' : false;
          const senderName = sender ? sender.nickname : '未知';

          let title, body;
          if (isGroup) {
            title = chatName;
            body = `${senderName}: ${processedContent.substring(0, 100)}`;
          } else {
            title = senderName;
            body = processedContent.substring(0, 100);
          }

          // Display content type for non-text messages
          if (messageType !== 'TEXT') {
            const typeLabel = { IMAGE: '[图片]', VIDEO: '[视频]', FILE: '[文件]', CARD: '[群公告]' };
            body = typeLabel[messageType] || '[消息]';
          }

          this.getui.sendPushToSingle(pushCid, title, body, {
            chatId: chatId,
            senderId: ws.userId,
          }).catch(err => {
            console.error('[WS] Push notification failed:', err.message);
          });
        }
      }
    });

    // Send @all remaining info back to sender
    if (atAll) {
      ws.send(JSON.stringify({ type: 'at_all_info', remaining: atAllRemaining }));
    }
  }

  handleRecall(ws, msg) {
    if (!ws.userId) return;

    const { chatId, messageId } = msg;
    if (!chatId || !messageId) return;

    // Verify the sender owns the message
    const originalMsg = db.findMessageById(messageId);
    if (!originalMsg || originalMsg.sender_id !== ws.userId) return;

    // Remove message from server storage
    db.removeMessage(messageId);

    // Log message recall
    logger.user.recallMessage(ws.userId, messageId);

    // Forward RECALL to all OTHER members of the chat
    const memberIds = db.getChatMembers(chatId);
    memberIds.forEach(uid => {
      if (uid === ws.userId) return;
      const client = this.clients.get(uid);
      if (client && client.readyState === WebSocket.OPEN) {
        client.send(JSON.stringify({
          type: 'RECALL',
          chatId,
          messageId
        }));
      } else {
        // Queue recall for offline user
        db.addToRecallQueue(uid, chatId, messageId);
      }
      // Also remove from offline message queue if present
      db.offlineQueue = db.offlineQueue.filter(
        q => !(q.recipient_id === uid && q.message && q.message.id === messageId)
      );
    });
  }

  handleTyping(ws, msg) {
    if (!ws.userId) return;
    const { chatId } = msg;

    const memberIds = db.getChatMembers(chatId).filter(uid => uid !== ws.userId);
    memberIds.forEach(uid => {
      const client = this.clients.get(uid);
      if (client && client.readyState === WebSocket.OPEN) {
        client.send(JSON.stringify({
          type: 'typing',
          chatId,
          userId: ws.userId
        }));
      }
    });
  }

  handleMessageAck(ws, msg) {
    const { messageId, status } = msg;
    db.updateMessageStatus(messageId, status);
  }

  handleFriendRequest(ws, msg) {
    if (!ws.userId) return;
    const { toUserId } = msg;

    const client = this.clients.get(toUserId);
    if (client && client.readyState === WebSocket.OPEN) {
      client.send(JSON.stringify({
        type: 'friend_request',
        fromUserId: ws.userId
      }));
    }
  }

  broadcastOnlineStatus(userId, status) {
    // Update user's online status in DB
    const user = db.findUserById(userId);
    if (user) user.online_status = status;

    // Notify friends about online status change
    const friendIds = db.getFriends(userId);
    friendIds.forEach(friendId => {
      const client = this.clients.get(friendId);
      if (client && client.readyState === WebSocket.OPEN) {
        client.send(JSON.stringify({
          type: 'online_status',
          userId,
          status
        }));
      }
    });
  }
}

module.exports = WebSocketManager;
