const express = require('express');
const db = require('../db');
const { authMiddleware } = require('../middleware/auth');

const router = express.Router();

// 获取聊天历史 - server doesn't store messages, return empty
router.get('/:chatId', authMiddleware, (req, res) => {
  const members = db.getChatMembers(req.params.chatId);
  if (!members.includes(req.userId)) {
    return res.status(403).json({ error: 'Not a member' });
  }
  // Messages are stored locally on each client device
  res.json({ messages: [] });
});

// 发送消息 - server only validates membership, actual sending via WebSocket
router.post('/:chatId', authMiddleware, (req, res) => {
  const members = db.getChatMembers(req.params.chatId);
  if (!members.includes(req.userId)) {
    return res.status(403).json({ error: 'Not a member' });
  }
  res.json({ success: true });
});

module.exports = router;
