const express = require('express');
const router = express.Router();
const { authMiddleware } = require('../middleware/auth');
const db = require('../db');

// GET /stickers - Get current user's stickers
router.get('/', authMiddleware, (req, res) => {
    const stickers = db.getUserStickers(req.userId);
    res.json({ stickers });
});

// POST /stickers - Add a sticker
router.post('/', authMiddleware, (req, res) => {
    const { url } = req.body;
    if (!url) return res.status(400).json({ error: 'URL required' });
    const sticker = db.addSticker(req.userId, url);
    res.json({ sticker });
});

// DELETE /stickers/:id - Delete a sticker
router.delete('/:id', authMiddleware, (req, res) => {
    const success = db.deleteSticker(req.params.id, req.userId);
    if (!success) return res.status(404).json({ error: 'Sticker not found' });
    res.json({ success: true });
});

module.exports = router;
