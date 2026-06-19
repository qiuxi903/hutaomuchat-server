const express = require('express');
const { v4: uuidv4 } = require('uuid');
const db = require('../db');
const { authMiddleware } = require('../middleware/auth');

const router = express.Router();

// 获取好友列表
router.get('/', authMiddleware, (req, res) => {
  const friendIds = db.getFriends(req.userId);
  const friends = friendIds.map(id => db.findUserById(id)).filter(Boolean);
  res.json({ friends });
});

// 发送好友申请
router.post('/request', authMiddleware, (req, res) => {
  const { to_user_id, message } = req.body;
  if (!to_user_id) {
    return res.status(400).json({ error: 'to_user_id is required' });
  }

  if (to_user_id === req.userId) {
    return res.status(400).json({ error: 'Cannot add yourself' });
  }

  if (db.areFriends(req.userId, to_user_id)) {
    return res.status(409).json({ error: 'Already friends' });
  }

  const targetUser = db.findUserById(to_user_id);
  // If target user has friend_verification disabled, auto-accept
  if (targetUser && targetUser.friend_verification === false) {
    db.addFriendship(req.userId, to_user_id);
    db.addFriendship(to_user_id, req.userId);
    // Create single chat
    const chat = db.createChat({
      id: uuidv4(),
      type: 'SINGLE',
      name: '',
      created_at: Date.now()
    });
    db.addChatMember(chat.id, req.userId);
    db.addChatMember(chat.id, to_user_id);
    return res.json({ success: true, auto_accepted: true });
  }

  const id = uuidv4();
  db.friendRequests.push({
    id,
    from_user_id: req.userId,
    to_user_id,
    message: message || '',
    status: 'PENDING',
    created_at: Date.now()
  });

  res.json({ success: true, request_id: id });
});

// 获取收到的好友申请
router.get('/requests', authMiddleware, (req, res) => {
  const requests = db.friendRequests.filter(r => r.to_user_id === req.userId && r.status === 'PENDING');
  const result = requests.map(r => {
    const fromUser = db.findUserById(r.from_user_id);
    return { ...r, from_user: fromUser };
  });
  res.json({ requests: result });
});

// 同意/拒绝好友申请
router.post('/requests/:id/respond', authMiddleware, (req, res) => {
  const { status } = req.body;
  const request = db.friendRequests.find(r => r.id === req.params.id && r.to_user_id === req.userId);
  if (!request) {
    return res.status(404).json({ error: 'Request not found' });
  }

  request.status = status;

  if (status === 'ACCEPTED') {
    db.addFriendship(req.userId, request.from_user_id);
    db.addFriendship(request.from_user_id, req.userId);
  }

  res.json({ success: true });
});

// 删除好友
router.delete('/:friendId', authMiddleware, (req, res) => {
  const { friendId } = req.params;
  if (!db.areFriends(req.userId, friendId)) {
    return res.status(404).json({ error: 'Not friends' });
  }

  // Remove bidirectional friendship
  db.removeFriendship(req.userId, friendId);

  // Remove single chats between the two users
  const chatsToRemove = db.chats.filter(c => {
    if (c.type !== 'SINGLE') return false;
    const members = db.getChatMembers(c.id);
    return members.includes(req.userId) && members.includes(friendId);
  });
  chatsToRemove.forEach(c => db.removeChatById(c.id));

  res.json({ success: true });
});

module.exports = router;
