const bcrypt = require('bcryptjs');
const { v4: uuidv4 } = require('uuid');
const db = require('../db');
const { generateToken } = require('../middleware/auth');
const express = require('express');
const { authMiddleware } = require('../middleware/auth');

const router = express.Router();

function generateUid() {
  const nextNum = db.users.length + 1;
  return `htm-${String(nextNum).padStart(3, '0')}`;
}

const defaultSignatures = [
  '这个人很懒，什么都没写~',
  '今天也要加油鸭！',
  '生活不止眼前的代码',
  '保持微笑，一切都会好的',
  '努力成为更好的自己',
  '愿你每天开心快乐',
  '世界那么大，我想去看看',
  '做自己喜欢的事',
  '平凡的日子里闪闪发光',
  '每天都要开开心心'
];

// 注册
router.post('/register', (req, res) => {
  const { phone, password, nickname } = req.body;
  if (!phone || !password || !nickname) {
    return res.status(400).json({ error: 'Phone, password and nickname are required' });
  }

  const existing = db.findUserByPhone(phone);
  if (existing) {
    return res.status(409).json({ error: 'Phone already registered' });
  }

  const user = {
    id: uuidv4(),
    uid: generateUid(),
    phone,
    password_hash: bcrypt.hashSync(password, 10),
    nickname,
    signature: defaultSignatures[Math.floor(Math.random() * defaultSignatures.length)],
    gender: 'UNKNOWN',
    region: '中国',
    avatar_url: '',
    background_url: '',
    background_original_url: '',
    friend_verification: true,
    phone_searchable: true,
    recommendable: true,
    online_status: 'OFFLINE',
    created_at: Date.now()
  };

  db.createUser(user);
  db.autoJoinGlobalChat(user.id);

  const safeUser = {
    id: user.id,
    uid: user.uid,
    nickname: user.nickname,
    signature: user.signature,
    gender: user.gender,
    region: user.region,
    avatar_url: user.avatar_url,
    background_url: user.background_url,
    background_original_url: user.background_original_url || '',
    friend_verification: user.friend_verification !== false,
    phone_searchable: user.phone_searchable !== false,
    recommendable: user.recommendable !== false,
    online_status: user.online_status,
    phone: user.phone
  };

  const token = generateToken(safeUser);
  res.json({ token, user: safeUser });
});

// 登录
router.post('/login', (req, res) => {
  const { phone, password } = req.body;
  if (!phone || !password) {
    return res.status(400).json({ error: 'Phone and password are required' });
  }

  const user = db.findUserByPhone(phone);
  if (!user) {
    return res.status(401).json({ error: 'Invalid credentials' });
  }

  const valid = bcrypt.compareSync(password, user.password_hash);
  if (!valid) {
    return res.status(401).json({ error: 'Invalid credentials' });
  }

  // Check if user is banned
  if (user.banned) {
    return res.status(403).json({ error: '账号已被封禁，请联系管理员' });
  }

  const safeUser = {
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
    recommendable: user.recommendable !== false,
    online_status: user.online_status
  };

  const token = generateToken(safeUser);
  res.json({ token, user: safeUser });
});

// 获取自己的资料
router.get('/me', authMiddleware, (req, res) => {
  const user = db.findUserById(req.userId);
  if (!user) {
    return res.status(404).json({ error: 'User not found' });
  }
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
    recommendable: user.recommendable !== false,
    online_status: user.online_status,
    phone: user.phone
  }});
});

// 更新资料
router.put('/profile', authMiddleware, (req, res) => {
  const { nickname, signature, gender, region, avatar_url, background_url, background_original_url, friend_verification, phone_searchable, recommendable } = req.body;
  const updates = {};
  if (nickname !== undefined) updates.nickname = nickname;
  if (signature !== undefined) updates.signature = signature;
  if (gender !== undefined) updates.gender = gender;
  if (region !== undefined) updates.region = region;
  if (avatar_url !== undefined) updates.avatar_url = avatar_url;
  if (background_url !== undefined) updates.background_url = background_url;
  if (background_original_url !== undefined) updates.background_original_url = background_original_url;
  if (friend_verification !== undefined) updates.friend_verification = friend_verification === true || friend_verification === 'true';
  if (phone_searchable !== undefined) updates.phone_searchable = phone_searchable === true || phone_searchable === 'true';
  if (recommendable !== undefined) updates.recommendable = recommendable === true || recommendable === 'true';
  const user = db.updateUser(req.userId, updates);
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
    recommendable: user.recommendable !== false,
    online_status: user.online_status,
    phone: user.phone
  }});
});

module.exports = router;
