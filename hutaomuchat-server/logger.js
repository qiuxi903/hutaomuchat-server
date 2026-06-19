/**
 * HutaomuChat 服务端日志模块
 *
 * 功能：
 * 1. 服务端命令行日志（系统日志）
 * 2. 客户端用户操作日志（用户行为日志）
 *
 * 日志文件存储在 logs/ 目录下：
 * - system-YYYY-MM-DD.log: 系统日志
 * - user-YYYY-MM-DD.log: 用户操作日志
 */

const fs = require('fs');
const path = require('path');

// 日志目录
const LOGS_DIR = path.join(__dirname, 'logs');

// 确保日志目录存在
if (!fs.existsSync(LOGS_DIR)) {
    fs.mkdirSync(LOGS_DIR, { recursive: true });
}

// 日志级别
const LOG_LEVELS = {
    DEBUG: 0,
    INFO: 1,
    WARN: 2,
    ERROR: 3
};

// 当前日志级别（可通过环境变量配置）
const CURRENT_LEVEL = LOG_LEVELS[process.env.LOG_LEVEL || 'INFO'] || LOG_LEVELS.INFO;

// 内存日志缓存（用于 API 查询）
const systemLogCache = [];
const userLogCache = [];
const MAX_CACHE_SIZE = 1000;

/**
 * 获取当前日期字符串 YYYY-MM-DD
 */
function getDateStr() {
    const now = new Date();
    return now.toISOString().split('T')[0];
}

/**
 * 获取当前时间字符串 HH:mm:ss.SSS
 */
function getTimeStr() {
    const now = new Date();
    return now.toTimeString().split(' ')[0] + '.' + String(now.getMilliseconds()).padStart(3, '0');
}

/**
 * 格式化日志消息
 */
function formatLogMessage(level, message, data = null) {
    const time = getTimeStr();
    let logLine = `[${time}] [${level}] ${message}`;
    if (data) {
        logLine += ' ' + JSON.stringify(data);
    }
    return logLine;
}

/**
 * 写入日志文件
 */
function writeToFile(filename, logLine) {
    const dateStr = getDateStr();
    const filePath = path.join(LOGS_DIR, `${filename}-${dateStr}.log`);
    const fullLog = logLine + '\n';

    try {
        fs.appendFileSync(filePath, fullLog, 'utf8');
    } catch (err) {
        console.error('写入日志文件失败:', err.message);
    }
}

/**
 * 添加到缓存
 */
function addToCache(cache, entry) {
    cache.push(entry);
    if (cache.length > MAX_CACHE_SIZE) {
        cache.shift(); // 移除最旧的日志
    }
}

/**
 * 系统日志（服务端命令行日志）
 */
const system = {
    debug(message, data = null) {
        if (CURRENT_LEVEL <= LOG_LEVELS.DEBUG) {
            const logLine = formatLogMessage('DEBUG', message, data);
            console.log(`\x1b[90m${logLine}\x1b[0m`);
            writeToFile('system', logLine);
            addToCache(systemLogCache, { time: new Date().toISOString(), level: 'DEBUG', message, data });
        }
    },

    info(message, data = null) {
        if (CURRENT_LEVEL <= LOG_LEVELS.INFO) {
            const logLine = formatLogMessage('INFO', message, data);
            console.log(`\x1b[36m${logLine}\x1b[0m`);
            writeToFile('system', logLine);
            addToCache(systemLogCache, { time: new Date().toISOString(), level: 'INFO', message, data });
        }
    },

    warn(message, data = null) {
        if (CURRENT_LEVEL <= LOG_LEVELS.WARN) {
            const logLine = formatLogMessage('WARN', message, data);
            console.warn(`\x1b[33m${logLine}\x1b[0m`);
            writeToFile('system', logLine);
            addToCache(systemLogCache, { time: new Date().toISOString(), level: 'WARN', message, data });
        }
    },

    error(message, data = null) {
        if (CURRENT_LEVEL <= LOG_LEVELS.ERROR) {
            const logLine = formatLogMessage('ERROR', message, data);
            console.error(`\x1b[31m${logLine}\x1b[0m`);
            writeToFile('system', logLine);
            addToCache(systemLogCache, { time: new Date().toISOString(), level: 'ERROR', message, data });
        }
    }
};

/**
 * 用户操作日志（客户端用户行为日志）
 */
const user = {
    /**
     * 记录用户操作
     * @param {string} userId - 用户 ID
     * @param {string} action - 操作类型
     * @param {object} details - 操作详情
     */
    log(userId, action, details = {}) {
        const entry = {
            time: new Date().toISOString(),
            userId,
            action,
            ...details
        };

        const logLine = formatLogMessage('USER', `[${userId}] ${action}`, details);
        writeToFile('user', logLine);
        addToCache(userLogCache, entry);

        // 同时输出到控制台（灰色）
        console.log(`\x1b[90m[USER] ${logLine}\x1b[0m`);
    },

    // 常用操作快捷方法
    login(userId, ip) {
        this.log(userId, 'LOGIN', { ip });
    },

    logout(userId) {
        this.log(userId, 'LOGOUT');
    },

    register(userId, phone) {
        this.log(userId, 'REGISTER', { phone });
    },

    sendMessage(userId, chatId, messageType) {
        this.log(userId, 'SEND_MESSAGE', { chatId, messageType });
    },

    recallMessage(userId, messageId) {
        this.log(userId, 'RECALL_MESSAGE', { messageId });
    },

    createChat(userId, chatId, isGroup) {
        this.log(userId, 'CREATE_CHAT', { chatId, isGroup });
    },

    joinChat(userId, chatId) {
        this.log(userId, 'JOIN_CHAT', { chatId });
    },

    leaveChat(userId, chatId) {
        this.log(userId, 'LEAVE_CHAT', { chatId });
    },

    addFriend(userId, friendId) {
        this.log(userId, 'ADD_FRIEND', { friendId });
    },

    deleteFriend(userId, friendId) {
        this.log(userId, 'DELETE_FRIEND', { friendId });
    },

    postMoment(userId, momentId, scope) {
        this.log(userId, 'POST_MOMENT', { momentId, scope });
    },

    uploadFile(userId, filename, size) {
        this.log(userId, 'UPLOAD_FILE', { filename, size });
    },

    updateProfile(userId, fields) {
        this.log(userId, 'UPDATE_PROFILE', { fields });
    },

    updateGroupAvatar(userId, chatId) {
        this.log(userId, 'UPDATE_GROUP_AVATAR', { chatId });
    },

    postAnnouncement(userId, chatId) {
        this.log(userId, 'POST_ANNOUNCEMENT', { chatId });
    },

    atAll(userId, chatId) {
        this.log(userId, 'AT_ALL', { chatId });
    }
};

/**
 * 获取系统日志
 * @param {number} limit - 返回条数
 * @param {string} level - 日志级别过滤
 * @returns {Array}
 */
function getSystemLogs(limit = 100, level = null) {
    let logs = [...systemLogCache].reverse();
    if (level) {
        logs = logs.filter(log => log.level === level.toUpperCase());
    }
    return logs.slice(0, limit);
}

/**
 * 获取用户操作日志
 * @param {number} limit - 返回条数
 * @param {string} userId - 用户 ID 过滤
 * @param {string} action - 操作类型过滤
 * @returns {Array}
 */
function getUserLogs(limit = 100, userId = null, action = null) {
    let logs = [...userLogCache].reverse();
    if (userId) {
        logs = logs.filter(log => log.userId === userId);
    }
    if (action) {
        logs = logs.filter(log => log.action === action.toUpperCase());
    }
    return logs.slice(0, limit);
}

/**
 * 读取日志文件内容
 * @param {string} type - 日志类型 (system/user)
 * @param {string} date - 日期 YYYY-MM-DD
 * @param {number} lines - 返回最后 N 行
 * @returns {string}
 */
function getLogFileContent(type = 'system', date = null, lines = 200) {
    if (!date) {
        date = getDateStr();
    }

    const filename = `${type}-${date}.log`;
    const filePath = path.join(LOGS_DIR, filename);

    try {
        if (!fs.existsSync(filePath)) {
            return `日志文件不存在: ${filename}`;
        }

        const content = fs.readFileSync(filePath, 'utf8');
        const allLines = content.split('\n').filter(line => line.trim());
        const lastLines = allLines.slice(-lines);
        return lastLines.join('\n');
    } catch (err) {
        return `读取日志文件失败: ${err.message}`;
    }
}

/**
 * 获取可用的日志文件列表
 * @returns {object}
 */
function getLogFiles() {
    try {
        const files = fs.readdirSync(LOGS_DIR);
        const systemFiles = files.filter(f => f.startsWith('system-')).sort().reverse();
        const userFiles = files.filter(f => f.startsWith('user-')).sort().reverse();
        return { system: systemFiles, user: userFiles };
    } catch (err) {
        return { system: [], user: [] };
    }
}

/**
 * 清理旧日志文件
 * @param {number} daysToKeep - 保留天数
 */
function cleanOldLogs(daysToKeep = 7) {
    try {
        const files = fs.readdirSync(LOGS_DIR);
        const cutoffDate = new Date();
        cutoffDate.setDate(cutoffDate.getDate() - daysToKeep);

        let deletedCount = 0;
        files.forEach(file => {
            const match = file.match(/\d{4}-\d{2}-\d{2}/);
            if (match) {
                const fileDate = new Date(match[0]);
                if (fileDate < cutoffDate) {
                    fs.unlinkSync(path.join(LOGS_DIR, file));
                    deletedCount++;
                }
            }
        });

        if (deletedCount > 0) {
            system.info(`清理了 ${deletedCount} 个旧日志文件`);
        }
    } catch (err) {
        system.error('清理旧日志失败', { error: err.message });
    }
}

// 每天清理一次旧日志
setInterval(cleanOldLogs, 24 * 60 * 60 * 1000);

module.exports = {
    system,
    user,
    getSystemLogs,
    getUserLogs,
    getLogFileContent,
    getLogFiles,
    cleanOldLogs,
    LOG_LEVELS
};
