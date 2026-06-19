const express = require('express');
const db = require('../db');
const { authMiddleware } = require('../middleware/auth');

const router = express.Router();

// Register or update push token (GeTui CID)
// POST /api/push/token  { cid: "..." }
router.post('/token', authMiddleware, (req, res) => {
  const { cid } = req.body;
  if (!cid) {
    return res.status(400).json({ error: 'cid is required' });
  }

  db.setPushToken(req.user.id, cid, 'android');
  console.log(`[Push] Token registered for user ${req.user.id}: ${cid.substring(0, 12)}...`);
  res.json({ success: true });
});

// Remove push token (on logout)
// DELETE /api/push/token
router.delete('/token', authMiddleware, (req, res) => {
  db.removePushToken(req.user.id);
  console.log(`[Push] Token removed for user ${req.user.id}`);
  res.json({ success: true });
});

module.exports = router;
