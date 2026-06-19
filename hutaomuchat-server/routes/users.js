const bcrypt = require('bcryptjs');
const { v4: uuidv4 } = require('uuid');
const db = require('../db');
const { generateToken } = require('../middleware/auth');
const express = require('express');
const { authMiddleware } = require('../middleware/auth');

const router = express.Router();

// 搜索用户
router.get('/search', authMiddleware, (req, res) => {
  const { q } = req.query;
  if (!q) return res.status(400).json({ error: 'Query required' });

  const friendIds = new Set(
    db.friendships
      .filter(f => f.user_id === req.userId || f.friend_id === req.userId)
      .map(f => f.user_id === req.userId ? f.friend_id : f.user_id)
  );

  const users = db.users.filter(u => {
    if (u.id === req.userId) return false;
    // If searching by exact phone, respect phone_searchable setting
    if (u.phone === q && u.phone_searchable === false) return false;
    return u.uid === q || u.phone === q || u.nickname.includes(q);
  }).map(u => ({ ...u, is_friend: friendIds.has(u.id) }));
  res.json({ users });
});

// GET /users/recommended - Get random recommended users (for friend suggestions)
router.get('/recommended', authMiddleware, (req, res) => {
  // Exclude friends from recommendations
  const friendIds = db.friendships
    .filter(f => f.user_id === req.userId || f.friend_id === req.userId)
    .map(f => f.user_id === req.userId ? f.friend_id : f.user_id);
  const users = db.getRecommendedUsers(req.userId, friendIds);
  const safeUsers = users.map(u => ({
    id: u.id,
    uid: u.uid,
    nickname: u.nickname,
    signature: u.signature,
    gender: u.gender,
    region: u.region,
    avatar_url: u.avatar_url || '',
    background_url: u.background_url || '',
    online_status: u.online_status
  }));
  res.json({ users: safeUsers });
});

// 获取指定用户
router.get('/:id', authMiddleware, (req, res) => {
  const user = db.findUserById(req.params.id);
  if (!user) return res.status(404).json({ error: 'User not found' });

  res.json({ user: {
    id: user.id,
    uid: user.uid,
    nickname: user.nickname,
    signature: user.signature,
    gender: user.gender,
    region: user.region,
    avatar_url: user.avatar_url,
    background_url: user.background_url || '',
    background_original_url: user.background_original_url || '',
    friend_verification: user.friend_verification !== false,
    phone_searchable: user.phone_searchable !== false,
    online_status: user.online_status
  }});
});

module.exports = router;
