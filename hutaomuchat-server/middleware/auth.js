const jwt = require('jsonwebtoken');

const JWT_SECRET = process.env.JWT_SECRET || 'securechat-dev-secret-key-change-in-production';

function authMiddleware(req, res, next) {
  const authHeader = req.headers['authorization'];
  if (!authHeader) {
    return res.status(401).json({ error: 'Missing authorization header' });
  }

  const token = authHeader.startsWith('Bearer ') ? authHeader.slice(7) : authHeader;
  try {
    const decoded = jwt.verify(token, JWT_SECRET);
    req.userId = decoded.userId;
    req.uid = decoded.uid;

    // Check if user is banned
    try {
      const db = require('../db');
      const user = db.findUserById(decoded.userId);
      if (user && user.banned) {
        return res.status(403).json({ error: '账号已被封禁' });
      }
    } catch (e) { /* db not available for admin routes */ }

    next();
  } catch (err) {
    return res.status(401).json({ error: 'Invalid token' });
  }
}

function generateToken(user) {
  return jwt.sign(
    { userId: user.id, uid: user.uid, nickname: user.nickname },
    JWT_SECRET,
    { expiresIn: '7d' }
  );
}

module.exports = { authMiddleware, generateToken, JWT_SECRET };
