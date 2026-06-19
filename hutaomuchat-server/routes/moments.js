const express = require('express');
const { v4: uuidv4 } = require('uuid');
const db = require('../db');
const { authMiddleware } = require('../middleware/auth');

const router = express.Router();

// 获取朋友圈
router.get('/', authMiddleware, (req, res) => {
  const moments = db.getMoments().map(m => {
    const user = db.findUserById(m.user_id);
    const likes = db.getMomentLikes(m.id).map(l => db.findUserById(l.user_id)).filter(Boolean);
    const comments = db.getMomentComments(m.id);
    return { ...m, user, likes, comments };
  });
  res.json({ moments });
});

// 获取好友朋友圈
router.get('/friends', authMiddleware, (req, res) => {
  const moments = db.getFriendMoments(req.userId).map(m => {
    const user = db.findUserById(m.user_id);
    const likes = db.getMomentLikes(m.id).map(l => db.findUserById(l.user_id)).filter(Boolean);
    const comments = db.getMomentComments(m.id);
    return { ...m, user, likes, comments };
  });
  res.json({ moments });
});

// 发动态
router.post('/', authMiddleware, (req, res) => {
  const { content, images, scope } = req.body;
  if (!content) return res.status(400).json({ error: 'Content required' });

  const moment = {
    id: uuidv4(),
    user_id: req.userId,
    content,
    images: JSON.stringify(images || []),
    scope: scope || 'public', // 'public' for 用户圈, 'friends' for 朋友圈
    created_at: Date.now()
  };

  db.addMoment(moment);
  const user = db.findUserById(req.userId);
  res.json({ moment: { ...moment, user, likes: [], comments: [] } });
});

// 点赞
router.post('/:id/like', authMiddleware, (req, res) => {
  db.addMomentLike(req.params.id, req.userId);
  res.json({ success: true });
});

// 取消点赞
router.delete('/:id/like', authMiddleware, (req, res) => {
  db.momentLikes = db.momentLikes.filter(l => !(l.moment_id === req.params.id && l.user_id === req.userId));
  res.json({ success: true });
});

// 评论
router.post('/:id/comment', authMiddleware, (req, res) => {
  const { content, image_url } = req.body;
  if (!content && !image_url) return res.status(400).json({ error: 'Content or image required' });

  const user = db.findUserById(req.userId);
  const comment = {
    id: uuidv4(),
    moment_id: req.params.id,
    user_id: req.userId,
    content: content || '',
    image_url: image_url || null,
    nickname: user ? user.nickname : 'Unknown',
    created_at: Date.now()
  };

  db.addMomentComment(comment);
  res.json({ comment });
});

module.exports = router;
