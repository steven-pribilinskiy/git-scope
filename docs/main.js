// ===============================
// ANALYTICS SETUP
// ===============================
const analytics = {
    events: [],
    track: function (name, data = {}) {
        const event = { name, data, time: new Date().toISOString() };
        this.events.push(event);
        console.log('[Analytics]', name, data);
    }
};

analytics.track('page_view', { page: window.location.pathname });

// ===============================
// GOATCOUNTER EVENT TRACKING
// ===============================
function trackEvent(eventName, title) {
    if (window.goatcounter && window.goatcounter.count) {
        window.goatcounter.count({
            path: eventName,
            title: title || eventName,
            event: true
        });
    }
}

// ===============================
// COPY COMMAND
// ===============================
function copyInstallCommand() {
    const command = "brew tap Bharath-code/tap && brew install git-scope";

    // Fallback for non-secure contexts
    if (!navigator.clipboard) {
        const ta = document.createElement('textarea');
        ta.value = command;
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        document.body.removeChild(ta);
        showCopyFeedback();
        return;
    }

    navigator.clipboard.writeText(command).then(() => {
        showCopyFeedback();
        trackEvent('command-copied', 'Copied install command');
    }).catch(err => {
        console.error('Failed to copy: ', err);
    });
}

function showCopyFeedback() {
    const icon = document.getElementById('copy-icon');
    if (!icon) return;

    const originalHTML = icon.innerHTML;

    // Change to Checkmark
    icon.innerHTML = '<polyline points="20 6 9 17 4 12"></polyline>';
    icon.style.opacity = '1';
    icon.style.color = '#27C93F'; // Green from theme

    setTimeout(() => {
        icon.innerHTML = originalHTML;
        icon.style.opacity = '0.5';
        icon.style.color = 'currentColor';
    }, 2000);
}

function copyScriptCommand() {
    const command = "curl -sSL https://raw.githubusercontent.com/Bharath-code/git-scope/main/scripts/install.sh | sh";

    // Fallback for non-secure contexts
    if (!navigator.clipboard) {
        const ta = document.createElement('textarea');
        ta.value = command;
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        document.body.removeChild(ta);
        showScriptCopyFeedback();
        trackEvent('script-command-copied', 'Copied curl install command');
        return;
    }

    navigator.clipboard.writeText(command).then(() => {
        showScriptCopyFeedback();
        trackEvent('script-command-copied', 'Copied curl install command');
    }).catch(err => {
        console.error('Failed to copy: ', err);
    });
}

function showScriptCopyFeedback() {
    const icon = document.getElementById('script-copy-icon');
    if (!icon) return;

    const originalHTML = icon.innerHTML;

    // Change to Checkmark
    icon.innerHTML = '<polyline points="20 6 9 17 4 12"></polyline>';
    icon.style.opacity = '1';
    icon.style.color = '#27C93F'; // Green from theme

    setTimeout(() => {
        icon.innerHTML = originalHTML;
        icon.style.opacity = '0.5';
        icon.style.color = 'currentColor';
    }, 2000);
}

// ===============================
// CLICK TRACKING
// ===============================
document.querySelector('.support-btn')?.addEventListener('click', () => {
    trackEvent('sponsor-click', 'Clicked Sponsor Button in Nav');
});

document.getElementById('nav-github-link')?.addEventListener('click', () => {
    trackEvent('github-star-click', 'Clicked GitHub Stars in Nav');
});

document.getElementById('hero-install-options')?.addEventListener('click', () => {
    trackEvent('install-options-click', 'Clicked View Windows/Linux Install');
});

document.getElementById('hero-cta-star')?.addEventListener('click', () => {
    trackEvent('hero-cta-star-click', 'Clicked Hero Star CTA');
});

document.getElementById('hero-cta-features')?.addEventListener('click', () => {
    trackEvent('hero-cta-features-click', 'Clicked Hero Features CTA');
});

document.getElementById('features-view-all')?.addEventListener('click', () => {
    trackEvent('features-view-all-click', 'Clicked View All Features');
});

document.querySelectorAll('footer a').forEach(link => {
    link.addEventListener('click', () => {
        const linkName = link.textContent.trim();
        // Clean up SVG content if present in link text
        const cleanName = linkName || link.getAttribute('aria-label') || 'icon';
        trackEvent('footer-click-' + cleanName.toLowerCase().replace(/[^a-z0-9]+/g, '-'), 'Clicked ' + cleanName + ' in footer');
    });
});

// ===============================
// SCROLL DEPTH TRACKING
// ===============================
let maxScroll = 0;
let scrollMilestones = { 25: false, 50: false, 75: false, 100: false };
window.addEventListener('scroll', () => {
    const scrollPercent = Math.round((window.scrollY / (document.body.scrollHeight - window.innerHeight)) * 100);
    if (scrollPercent > maxScroll) {
        maxScroll = scrollPercent;
        [25, 50, 75, 100].forEach(milestone => {
            if (scrollPercent >= milestone && !scrollMilestones[milestone]) {
                scrollMilestones[milestone] = true;
                trackEvent('scroll-' + milestone + '-percent', 'Scrolled ' + milestone + '% of page');
            }
        });
    }
});

// ===============================
// SESSION SUMMARY
// ===============================
window.addEventListener('beforeunload', () => {
    analytics.track('session_end', {
        maxScroll: maxScroll,
        totalEvents: analytics.events.length
    });
});

// ===============================
// SCROLL REVEAL (IntersectionObserver)
// ===============================
const revealElements = document.querySelectorAll('.reveal');
const revealObserver = new IntersectionObserver((entries) => {
    entries.forEach(entry => {
        if (entry.isIntersecting) {
            entry.target.classList.add('revealed');
            // Also reveal staggered children
            const staggerChildren = entry.target.querySelectorAll('.reveal-stagger > *');
            staggerChildren.forEach((child, index) => {
                setTimeout(() => {
                    child.style.opacity = '1';
                    child.style.transform = 'translateY(0)';
                }, index * 100);
            });
        }
    });
}, {
    threshold: 0.1,
    rootMargin: '0px 0px -50px 0px'
});

revealElements.forEach(el => revealObserver.observe(el));

// Also handle reveal-stagger children initial state
document.querySelectorAll('.reveal-stagger > *').forEach(child => {
    child.style.opacity = '0';
    child.style.transform = 'translateY(20px)';
    child.style.transition = 'opacity 0.5s ease, transform 0.5s ease';
});

// ===============================
// HAMBURGER MENU
// ===============================
const hamburger = document.getElementById('hamburger');
const mobileNav = document.getElementById('mobileNav');

if (hamburger && mobileNav) {
    hamburger.addEventListener('click', () => {
        hamburger.classList.toggle('active');
        mobileNav.classList.toggle('open');
        document.body.style.overflow = mobileNav.classList.contains('open') ? 'hidden' : '';
    });

    // Close menu when clicking nav links
    mobileNav.querySelectorAll('a').forEach(link => {
        link.addEventListener('click', () => {
            hamburger.classList.remove('active');
            mobileNav.classList.remove('open');
            document.body.style.overflow = '';
        });
    });
}

// ===============================
// STICKY CTA PULSE (Mobile)
// ===============================
const mobileFooterBtn = document.querySelector('.mobile-footer-btn');
if (mobileFooterBtn) {
    // Add pulse class after a short delay to draw attention
    setTimeout(() => {
        mobileFooterBtn.classList.add('pulse');
    }, 2000);

    // Remove pulse after animation completes (3 cycles * 2s = 6s)
    setTimeout(() => {
        mobileFooterBtn.classList.remove('pulse');
    }, 8000);
}

// ===============================
// SKELETON LOADER
// ===============================
document.querySelectorAll('.skeleton-wrapper img').forEach(img => {
    if (img.complete) {
        img.closest('.skeleton-wrapper')?.classList.add('loaded');
    } else {
        img.addEventListener('load', () => {
            img.closest('.skeleton-wrapper')?.classList.add('loaded');
        });
    }
});

// ===============================
// KONAMI CODE EASTER EGG
// ===============================
const konamiCode = ['ArrowUp', 'ArrowUp', 'ArrowDown', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'ArrowLeft', 'ArrowRight', 'KeyB', 'KeyA'];
let konamiIndex = 0;

document.addEventListener('keydown', (e) => {
    if (e.code === konamiCode[konamiIndex]) {
        konamiIndex++;
        if (konamiIndex === konamiCode.length) {
            // Easter egg triggered!
            document.body.classList.add('konami-active');
            trackEvent('konami-triggered', 'User triggered Konami code easter egg');

            // Show a fun message
            const easterEgg = document.createElement('div');
            easterEgg.innerHTML = '🎮 You found the easter egg! 🎉';
            easterEgg.style.cssText = `
                position: fixed;
                top: 50%;
                left: 50%;
                transform: translate(-50%, -50%);
                background: linear-gradient(135deg, #7C3AED, #EC4899);
                color: white;
                padding: 2rem 3rem;
                border-radius: 16px;
                font-size: 1.5rem;
                font-weight: 700;
                z-index: 10000;
                animation: fadeInUp 0.5s ease;
                box-shadow: 0 20px 50px rgba(139, 92, 246, 0.5);
            `;
            document.body.appendChild(easterEgg);

            // Remove after 3 seconds
            setTimeout(() => {
                easterEgg.style.opacity = '0';
                easterEgg.style.transition = 'opacity 0.5s ease';
                setTimeout(() => {
                    easterEgg.remove();
                    document.body.classList.remove('konami-active');
                }, 500);
            }, 3000);

            konamiIndex = 0;
        }
    } else {
        konamiIndex = 0;
    }
});
